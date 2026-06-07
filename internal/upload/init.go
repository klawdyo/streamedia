package upload

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// initRequest representa o corpo JSON esperado em POST /upload/init.
// video_id é opcional: se informado, deve ser um UUID bem-formado; se omitido,
// o servidor gera um UUID v7.
type initRequest struct {
	VideoID           string `json:"video_id"`
	DeclaredSizeBytes int64  `json:"declared_size_bytes"`
}

// InitHandler trata a rota de inicialização de upload (POST /upload/init).
type InitHandler struct {
	cfg *config.Config
	db  *sql.DB
}

// NewInitHandler cria um novo handler de inicialização de upload.
func NewInitHandler(cfg *config.Config, db *sql.DB) *InitHandler {
	return &InitHandler{cfg: cfg, db: db}
}

// ServeHTTP processa a inicialização de um upload: valida autenticação do
// backend, registra o vídeo, gera o token de upload e devolve a URL de upload.
//
// Dois fluxos de autenticação coexistem (T33, issue #6):
//
//   - Escopado a projeto: header X-Project-Key com a chave mestra do
//     projeto em texto puro. O servidor calcula o hash e resolve o projeto
//     (models.GetProjectByMasterKeyHash) — análogo ao Bearer do
//     ADMIN_TOKEN, e evita reter/recuperar a chave em texto puro só para
//     validar HMAC. Gera um token de upload de vida curta
//     (UploadTokenScopedTTL, ~15-20min) vinculado a project_id + video_id.
//   - Legado/global: header X-Upload-Auth com HMAC sobre o corpo, assinado
//     com UPLOAD_TOKEN_SECRET — comportamento preexistente, preservado para
//     compatibilidade com instalações sem projetos. project_id fica nil.
func (h *InitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Lê o corpo da requisição com limite de 1MB.
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respondError(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}

	// 2. Resolve a autenticação: chave de projeto (X-Project-Key) tem
	// prioridade sobre o HMAC global legado (X-Upload-Auth). project é nil
	// no fluxo legado — o vídeo e o token são criados sem projeto associado.
	var project *models.Project
	var projectKey string
	if projectKey = r.Header.Get("X-Project-Key"); projectKey != "" {
		project, err = models.GetProjectByMasterKeyHash(h.db, models.HashMasterKey(projectKey))
		if err != nil {
			if err == sql.ErrNoRows {
				respondError(w, http.StatusUnauthorized, "Chave de projeto inválida.")
				return
			}
			respondError(w, http.StatusInternalServerError, "Falha ao validar a chave de projeto.")
			return
		}
	} else {
		sig := r.Header.Get("X-Upload-Auth")
		if sig == "" || !auth.ValidateBackendAuth(h.cfg.UploadTokenSecret, bodyBytes, sig) {
			respondError(w, http.StatusUnauthorized, "Autorização inválida.")
			return
		}
	}

	// 3. Faz o parse do JSON do corpo.
	var req initRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido.")
		return
	}

	// 4. Resolve video_id: se informado, valida formato; se ausente, gera UUID v7.
	videoID := req.VideoID
	if videoID == "" {
		// Cliente não informou video_id — o servidor gera um UUID v7.
		// O sistema sempre privilegia v7 ao gerar ids: é ordenável por tempo,
		// o que melhora localidade no índice do SQLite.
		var err error
		videoID, err = models.NewVideoID()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Falha ao gerar video_id.")
			return
		}
	} else if !models.IsValidVideoIDFormat(videoID) {
		// video_id informado mas não é um UUID bem-formado — rejeita.
		// Continua barrando path traversal: só UUIDs passam.
		respondError(w, http.StatusBadRequest, "video_id inválido: deve ser um UUID bem-formado.")
		return
	}

	// 5. Valida declared_size_bytes: deve ser positivo e dentro do limite.
	if req.DeclaredSizeBytes <= 0 {
		respondError(w, http.StatusBadRequest, "declared_size_bytes deve ser maior que zero.")
		return
	}
	if req.DeclaredSizeBytes > h.cfg.MaxUploadSizeBytes {
		respondError(w, http.StatusRequestEntityTooLarge, "declared_size_bytes acima do limite permitido.")
		return
	}

	// 6. Resolve o vínculo de projeto (nil no fluxo legado) e insere o vídeo;
	// conflito de chave (UNIQUE) vira 409.
	var projectID *int64
	if project != nil {
		projectID = &project.ID
	}
	if err := models.InsertVideoForProject(h.db, videoID, req.DeclaredSizeBytes, projectID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			respondError(w, http.StatusConflict, "video_id já existe.")
			return
		}
		respondError(w, http.StatusInternalServerError, "Falha ao registrar o vídeo.")
		return
	}

	// 7. Gera e persiste o token de upload com expiração configurada.
	//
	// No fluxo escopado a projeto (T33, issue #6), o HMAC é assinado com a
	// própria chave mestra do projeto (em vez do segredo global) — reaproveita
	// auth.GenerateUploadToken e mantém o token verificável sem persistir a
	// chave em claro; e o TTL é bem mais curto (UploadTokenScopedTTL, ~15-20min,
	// "um único arquivo"), em vez do TTL global de horas.
	var token string
	var ttl time.Duration
	if project != nil {
		token = auth.GenerateUploadToken(projectKey, videoID)
		ttl = h.cfg.UploadTokenScopedTTL
	} else {
		token = auth.GenerateUploadToken(h.cfg.UploadTokenSecret, videoID)
		ttl = h.cfg.UploadTokenTTL
	}
	expiresAt := time.Now().Add(ttl)
	if err := models.InsertUploadTokenForProject(h.db, token, videoID, expiresAt, projectID); err != nil {
		respondError(w, http.StatusInternalServerError, "Falha ao registrar o token de upload.")
		return
	}

	// 8. Constrói a URL de upload respeitando proxies (X-Forwarded-*).
	scheme := "https"
	if r.TLS == nil {
		if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
			scheme = fwdProto
		} else {
			scheme = "http"
		}
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	uploadURL := fmt.Sprintf("%s://%s/files/%s", scheme, host, videoID)

	// 9. Responde 200 com video_id, URL de upload e o token.
	// video_id sempre está presente na resposta: gerado pelo servidor ou
	// ecoado do que o cliente informou.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"video_id":   videoID,
		"upload_url": uploadURL,
		"token":      token,
	})
}

// respondError escreve uma resposta de erro JSON com o status e mensagem dados.
func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}
