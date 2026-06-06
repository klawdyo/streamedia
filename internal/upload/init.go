package upload

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// uuidV4Re valida estritamente um UUID versão 4 (compilada uma única vez).
var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// initRequest representa o corpo JSON esperado em POST /upload/init.
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
func (h *InitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Lê o corpo da requisição com limite de 1MB.
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respondError(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}

	// 2. Valida o HMAC do header X-Upload-Auth sobre o corpo bruto.
	sig := r.Header.Get("X-Upload-Auth")
	if sig == "" || !auth.ValidateBackendAuth(h.cfg.UploadTokenSecret, bodyBytes, sig) {
		respondError(w, http.StatusUnauthorized, "Autorização inválida.")
		return
	}

	// 3. Faz o parse do JSON do corpo.
	var req initRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido.")
		return
	}

	// 4. Valida video_id como UUID v4 estrito (também barra path traversal).
	if !uuidV4Re.MatchString(req.VideoID) {
		respondError(w, http.StatusBadRequest, "video_id inválido: deve ser um UUID v4.")
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

	// 6. Insere o vídeo no banco; conflito de chave (UNIQUE) vira 409.
	if err := models.InsertVideo(h.db, req.VideoID, req.DeclaredSizeBytes); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			respondError(w, http.StatusConflict, "video_id já existe.")
			return
		}
		respondError(w, http.StatusInternalServerError, "Falha ao registrar o vídeo.")
		return
	}

	// 7. Gera e persiste o token de upload com expiração configurada.
	token := auth.GenerateUploadToken(h.cfg.UploadTokenSecret, req.VideoID)
	expiresAt := time.Now().Add(h.cfg.UploadTokenTTL)
	if err := models.InsertUploadToken(h.db, token, req.VideoID, expiresAt); err != nil {
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
	uploadURL := fmt.Sprintf("%s://%s/files/%s", scheme, host, req.VideoID)

	// 9. Responde 200 com a URL de upload e o token.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
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
