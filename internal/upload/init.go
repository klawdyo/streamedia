package upload

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/httputil"
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

// ServeHTTP processa a inicialização de um upload: valida a chave mestra do
// projeto via X-Project-Key, registra o vídeo, gera o token de upload e
// devolve a URL de upload.
//
// A partir da issue #10 (T49), X-Project-Key é OBRIGATÓRIO — o fluxo HMAC
// legado (X-Upload-Auth assinado com UPLOAD_TOKEN_SECRET) foi removido.
// uploads sem um projeto explícito usam a chave do projeto padrão "Default"
// (criado automaticamente na inicialização, ver EnsureDefaultProject).
func (h *InitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Lê o corpo da requisição com limite de 1MB.
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}

	// 2. Valida autenticação: X-Project-Key é obrigatório desde a T49.
	// A chave mestra do projeto é usada tanto para autenticar a requisição
	// quanto para assinar o token de upload gerado (HMAC com a própria chave).
	projectKey := r.Header.Get("X-Project-Key")
	if projectKey == "" {
		apiresponse.Error(w, http.StatusUnauthorized, "X-Project-Key é obrigatório.")
		return
	}

	project, err := models.GetProjectByMasterKeyHash(h.db, models.HashMasterKey(projectKey))
	if err != nil {
		if err == sql.ErrNoRows {
			apiresponse.Error(w, http.StatusUnauthorized, "Chave de projeto inválida.")
			return
		}
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao validar a chave de projeto.")
		return
	}

	// 3. Faz o parse do JSON do corpo.
	var req initRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "JSON inválido.")
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
			apiresponse.Error(w, http.StatusInternalServerError, "Falha ao gerar video_id.")
			return
		}
	} else if !models.IsValidVideoIDFormat(videoID) {
		// video_id informado mas não é um UUID bem-formado — rejeita.
		// Continua barrando path traversal: só UUIDs passam.
		apiresponse.Error(w, http.StatusBadRequest, "video_id inválido: deve ser um UUID bem-formado.")
		return
	}

	// 5. Valida declared_size_bytes: deve ser positivo e dentro do limite.
	if req.DeclaredSizeBytes <= 0 {
		apiresponse.Error(w, http.StatusBadRequest, "declared_size_bytes deve ser maior que zero.")
		return
	}
	if req.DeclaredSizeBytes > h.cfg.MaxUploadSizeBytes {
		apiresponse.Error(w, http.StatusRequestEntityTooLarge, "declared_size_bytes acima do limite permitido.")
		return
	}

	// 6. Insere o vídeo vinculado ao projeto resolvido.
	// O vínculo de projeto SEMPRE existe — todo upload pertence a um projeto.
	projectID := &project.ID
	if err := models.InsertVideoForProject(h.db, videoID, req.DeclaredSizeBytes, projectID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			apiresponse.Error(w, http.StatusConflict, "video_id já existe.")
			return
		}
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o vídeo.")
		return
	}

	// 7. Gera e persiste o token de upload assinado com a chave mestra do
	// projeto. TTL único (UPLOAD_TOKEN_TTL_SECONDS) desde a issue #10/T50.
	token := auth.GenerateUploadToken(projectKey, videoID)
	ttl := h.cfg.UploadTokenTTL
	expiresAt := time.Now().Add(ttl)
	if err := models.InsertUploadTokenForProject(h.db, token, videoID, expiresAt, projectID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o token de upload.")
		return
	}

	// 8. Constrói a URL de upload usando a função centralizada (httputil),
	// que resolve scheme/host a partir dos headers de proxy (X-Forwarded-*).
	uploadURL := httputil.PublicUploadURL(r, videoID)

	// 9. Responde 200 com video_id, URL de upload e o token.
	// video_id sempre está presente na resposta: gerado pelo servidor ou
	// ecoado do que o cliente informou.
	apiresponse.Success(w, http.StatusOK, map[string]string{
		"video_id":   videoID,
		"upload_url": uploadURL,
		"token":      token,
	})
}
