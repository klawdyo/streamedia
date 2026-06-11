package upload

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/httputil"
	"github.com/klawdyo/streamedia/internal/models"
)

// maxWebhookURLLen é o tamanho máximo aceito para a webhook_url customizada
// (issue #20). Limita o que será persistido na coluna videos.webhook_url.
const maxWebhookURLLen = 2048

// initRequest representa o corpo JSON esperado em POST /api/upload/init.
//   - tag: namespace organizacional do vídeo (obrigatório); normalizado por Slugify.
//   - video_id: opcional; se informado deve ser um UUID bem-formado; se omitido,
//     o servidor gera um UUID v7.
//   - webhook_url: opcional; URL HTTPS de destino dos webhooks deste vídeo
//     (issue #20). Omitido/vazio → usa a WEBHOOK_URL global. Quando informado,
//     deve ser uma URL HTTPS válida de no máximo 2048 caracteres.
type initRequest struct {
	Tag               string `json:"tag"`
	VideoID           string `json:"video_id"`
	DeclaredSizeBytes int64  `json:"declared_size_bytes"`
	WebhookURL        string `json:"webhook_url"`
}

// validateWebhookURL valida a URL de webhook customizada informada no
// upload/init (issue #20). Regras: HTTPS, formato de URL absoluta válido e no
// máximo maxWebhookURLLen caracteres. Espaços nas bordas são removidos. Retorna
// a URL normalizada e ok=true quando válida; ok=false indica que o valor
// informado é inválido (o chamador responde 400). String vazia NÃO chega aqui:
// o chamador trata "omitido" como "usar a URL global" antes de validar.
func validateWebhookURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxWebhookURLLen {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	// Exige HTTPS e um host — descarta esquemas não-HTTPS e URLs relativas.
	if u.Scheme != "https" || u.Host == "" {
		return "", false
	}
	return raw, true
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

	// 5.1 Resolve webhook_url customizado (issue #20). Omitido/vazio → fica ""
	// (o vídeo usará a WEBHOOK_URL global). Informado mas inválido → 400: é mais
	// seguro avisar o chamador do que silenciosamente enviar os eventos deste
	// vídeo para o destino global (vazamento entre tenants em cenário multi-tenant).
	webhookURL := ""
	if strings.TrimSpace(req.WebhookURL) != "" {
		normalized, ok := validateWebhookURL(req.WebhookURL)
		if !ok {
			apiresponse.Error(w, http.StatusBadRequest, "webhook_url inválido: deve ser uma URL HTTPS válida de até 2048 caracteres.")
			return
		}
		webhookURL = normalized
	}

	// 6. Insere o vídeo no namespace (tag), com a webhook_url customizada (ou "").
	if err := models.InsertVideoWithTagAndWebhook(h.db, videoID, req.DeclaredSizeBytes, tag, webhookURL); err != nil {
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
