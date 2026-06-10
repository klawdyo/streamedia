package upload

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/httputil"
	"github.com/klawdyo/streamedia/internal/models"
)

// initRequest representa o corpo JSON esperado em POST /api/upload/init.
//   - tag: namespace organizacional do vídeo (obrigatório); normalizado por Slugify.
//   - video_id: opcional; se informado deve ser um UUID bem-formado; se omitido,
//     o servidor gera um UUID v7.
type initRequest struct {
	Tag               string `json:"tag"`
	VideoID           string `json:"video_id"`
	DeclaredSizeBytes int64  `json:"declared_size_bytes"`
}

// InitHandler trata a rota de inicialização de upload (POST /api/upload/init).
// A autenticação (ROOT_TOKEN) é feita pelo middleware RootAuth no roteador.
type InitHandler struct {
	cfg *config.Config
	db  *sql.DB
}

// NewInitHandler cria um novo handler de inicialização de upload.
func NewInitHandler(cfg *config.Config, db *sql.DB) *InitHandler {
	return &InitHandler{cfg: cfg, db: db}
}

// ServeHTTP registra o vídeo no namespace (tag) informado, gera um token de
// upload efêmero e devolve a URL de upload TUS.
func (h *InitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Lê o corpo da requisição com limite de 1MB.
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		log.Printf("[upload] init: erro ao ler corpo da requisição: %v", err)
		apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}

	// 2. Faz o parse do JSON do corpo.
	var req initRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "JSON inválido.")
		return
	}

	// 3. Normaliza e valida a tag (namespace). Slugify também neutraliza
	// qualquer tentativa de path traversal (a tag vira diretório no disco).
	tag := models.Slugify(req.Tag)
	if tag == "" {
		apiresponse.Error(w, http.StatusBadRequest, "O campo 'tag' é obrigatório.")
		return
	}

	// 4. Resolve video_id: se informado, valida formato; se ausente, gera UUID v7.
	videoID := req.VideoID
	if videoID == "" {
		videoID, err = models.NewVideoID()
		if err != nil {
			log.Printf("[upload] init: erro ao gerar video_id: %v", err)
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

	// 6. Insere o vídeo no namespace (tag).
	if err := models.InsertVideoWithTag(h.db, videoID, req.DeclaredSizeBytes, tag); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			apiresponse.Error(w, http.StatusConflict, "video_id já existe.")
			return
		}
		log.Printf("[upload] init: erro ao registrar vídeo (video_id=%s, tag=%s, size=%d): %v", videoID, tag, req.DeclaredSizeBytes, err)
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o vídeo.")
		return
	}

	// 7. Gera e persiste o token de upload (string aleatória, purpose=upload).
	token, err := auth.GenerateToken()
	if err != nil {
		log.Printf("[upload] init: erro ao gerar token de upload (video_id=%s): %v", videoID, err)
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao gerar o token de upload.")
		return
	}
	expiresAt := time.Now().Add(h.cfg.UploadTokenTTL)
	if err := models.InsertAccessToken(h.db, token, videoID, models.PurposeUpload, expiresAt); err != nil {
		log.Printf("[upload] init: erro ao registrar token de upload (video_id=%s): %v", videoID, err)
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o token de upload.")
		return
	}

	// 8. Constrói a URL de upload usando a função centralizada (httputil).
	uploadURL := httputil.PublicUploadURL(r, videoID)

	// 9. Responde 200 com video_id, URL de upload e o token.
	apiresponse.Success(w, http.StatusOK, map[string]string{
		"video_id":   videoID,
		"tag":        tag,
		"upload_url": uploadURL,
		"token":      token,
	})
}
