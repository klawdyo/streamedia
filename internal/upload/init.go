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

// ServeHTTP processa a inicialização de um upload: resolve o projeto (explícito
// via X-Project-Key ou o projeto padrão "Default"), registra o vídeo, gera o
// token de upload e devolve a URL de upload.
//
// X-Project-Key é OPCIONAL — se ausente, o upload é associado ao projeto
// padrão "Default" (criado automaticamente na inicialização) e o token é
// assinado com UPLOAD_TOKEN_SECRET. Se presente, o token é assinado com a
// própria chave mestra do projeto.
func (h *InitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Lê o corpo da requisição com limite de 1MB.
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}

	// 2. Resolve o projeto: X-Project-Key explícito OU projeto padrão.
	// signingKey é o segredo usado para assinar o token de upload — a chave
	// mestra do projeto (quando X-Project-Key é informado) ou o secret global
	// (quando o upload cai no projeto padrão sem chave explícita).
	var project *models.Project
	var signingKey string
	if projectKey := r.Header.Get("X-Project-Key"); projectKey != "" {
		project, err = models.GetProjectByMasterKeyHash(h.db, models.HashMasterKey(projectKey))
		if err != nil {
			if err == sql.ErrNoRows {
				apiresponse.Error(w, http.StatusUnauthorized, "Chave de projeto inválida.")
				return
			}
			apiresponse.Error(w, http.StatusInternalServerError, "Falha ao validar a chave de projeto.")
			return
		}
		signingKey = projectKey
	} else {
		// Sem X-Project-Key: usa o projeto padrão (issue #10, T48).
		project, err = models.EnsureDefaultProject(h.db)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Falha ao resolver o projeto padrão.")
			return
		}
		signingKey = h.cfg.UploadTokenSecret
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
		var err error
		videoID, err = models.NewVideoID()
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Falha ao gerar video_id.")
			return
		}
	} else if !models.IsValidVideoIDFormat(videoID) {
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
	projectID := &project.ID
	if err := models.InsertVideoForProject(h.db, videoID, req.DeclaredSizeBytes, projectID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			apiresponse.Error(w, http.StatusConflict, "video_id já existe.")
			return
		}
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o vídeo.")
		return
	}

	// 7. Gera e persiste o token de upload assinado com signingKey.
	token := auth.GenerateUploadToken(signingKey, videoID)
	ttl := h.cfg.UploadTokenTTL
	expiresAt := time.Now().Add(ttl)
	if err := models.InsertUploadTokenForProject(h.db, token, videoID, expiresAt, projectID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o token de upload.")
		return
	}

	// 8. Constrói a URL de upload usando a função centralizada (httputil).
	uploadURL := httputil.PublicUploadURL(r, videoID)

	// 9. Responde 200 com video_id, URL de upload e o token.
	apiresponse.Success(w, http.StatusOK, map[string]string{
		"video_id":   videoID,
		"upload_url": uploadURL,
		"token":      token,
	})
}
