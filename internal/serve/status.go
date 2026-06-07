package serve

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// StatusResponse é a struct de resposta para GET /api/status/{video_id}.
type StatusResponse struct {
	VideoID           string    `json:"video_id"`
	Status            string    `json:"status"`
	DurationS         *int      `json:"duration_s"`
	Resolutions       []int     `json:"resolutions"`
	TranscodeAttempts int       `json:"transcode_attempts"`
	ErrorMessage      *string   `json:"error_message"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// StatusHandler serve a rota GET /api/status/{video_id} autenticada por HMAC.
type StatusHandler struct {
	cfg *config.Config
	db  *sql.DB
}

// NewStatusHandler cria um StatusHandler com a config e o banco informados.
func NewStatusHandler(cfg *config.Config, db *sql.DB) *StatusHandler {
	return &StatusHandler{cfg: cfg, db: db}
}

// ServeHTTP implementa o fluxo de autenticação e resposta para GET /api/status/{video_id}.
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extrai o video_id do path: /api/status/{videoID}
	// Split by "/" e pega o último segmento não vazio
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	var videoID string
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			videoID = parts[i]
			break
		}
	}

	if videoID == "" {
		apiresponse.Error(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	// 2. Valida o video_id como UUID v4 estrito.
	if !uuidV4Re.MatchString(videoID) {
		apiresponse.Error(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	// 3. Valida o HMAC da requisição usando X-Status-Auth header.
	signature := r.Header.Get("X-Status-Auth")
	if signature == "" {
		apiresponse.Error(w, http.StatusUnauthorized, "Header X-Status-Auth ausente.")
		return
	}

	// Valida a assinatura HMAC com o video_id como body
	if !auth.ValidateBackendAuth(h.cfg.UploadTokenSecret, []byte(videoID), signature) {
		apiresponse.Error(w, http.StatusUnauthorized, "Autenticação inválida.")
		return
	}

	// 4. Busca o vídeo no banco.
	video, err := models.GetVideo(h.db, videoID)
	if err == sql.ErrNoRows {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao consultar o vídeo.")
		return
	}

	// 5. Constrói a resposta.
	// DurationS é pointer: nil se 0 (vídeo não processado), caso contrário aponta para o valor
	var durationS *int
	if video.DurationS > 0 {
		durationS = &video.DurationS
	}

	// ErrorMessage é pointer: nil se vazio, caso contrário aponta para o valor
	var errorMessage *string
	if video.ErrorMessage != "" {
		errorMessage = &video.ErrorMessage
	}

	// Resolutions é um []int: se vazio ou nil, mantém como []int{}
	resolutions := video.Resolutions
	if resolutions == nil {
		resolutions = []int{}
	}

	resp := StatusResponse{
		VideoID:           video.VideoID,
		Status:            string(video.Status),
		DurationS:         durationS,
		Resolutions:       resolutions,
		TranscodeAttempts: video.TranscodeAttempts,
		ErrorMessage:      errorMessage,
		CreatedAt:         video.CreatedAt,
		UpdatedAt:         video.UpdatedAt,
	}

	// 6. Escreve a resposta JSON no envelope padrão.
	apiresponse.Success(w, http.StatusOK, resp)
}
