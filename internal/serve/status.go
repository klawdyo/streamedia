package serve

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/httputil"
	"github.com/klawdyo/streamedia/internal/models"
)

// StatusResponse é a struct de resposta para GET /api/status/{video_id}.
type StatusResponse struct {
	VideoID           string    `json:"video_id"`
	Status            string    `json:"status"`
	Tag               string    `json:"tag"`
	DurationS         *int      `json:"duration_s"`
	Resolutions       []int     `json:"resolutions"`
	TranscodeAttempts int       `json:"transcode_attempts"`
	ErrorMessage      *string   `json:"error_message"`
	// HasThumbnails indica se há ao menos um thumbnail (poster) gerado no disco
	// para o vídeo. Thumbnails são derivados do disco (não de coluna no banco),
	// então este campo é sempre coerente com o que a rota pública serve (issue #19).
	HasThumbnails bool `json:"has_thumbnails"`
	// Thumbnails mapeia cada resolução (como string) à URL pública do seu
	// thumbnail. Inclui apenas as resoluções cujo arquivo existe no disco.
	Thumbnails map[string]string `json:"thumbnails"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// StatusHandler serve a rota GET /api/status/{video_id}. A autenticação
// (ROOT_TOKEN) é feita pelo middleware RootAuth no roteador.
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

	// 3. Busca o vídeo no banco.
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

	// Thumbnails (poster) por resolução: derivados do disco (issue #19). Inclui
	// apenas as resoluções cujo arquivo thumb_<res>.jpg existe.
	thumbnails := h.collectThumbnails(r, video)

	resp := StatusResponse{
		VideoID:           video.VideoID,
		Status:            string(video.Status),
		Tag:               video.Tag,
		DurationS:         durationS,
		Resolutions:       resolutions,
		TranscodeAttempts: video.TranscodeAttempts,
		ErrorMessage:      errorMessage,
		HasThumbnails:     len(thumbnails) > 0,
		Thumbnails:        thumbnails,
		CreatedAt:         video.CreatedAt,
		UpdatedAt:         video.UpdatedAt,
	}

	// 6. Escreve a resposta JSON no envelope padrão.
	apiresponse.Success(w, http.StatusOK, resp)
}

// collectThumbnails verifica no disco quais thumbnails existem para o vídeo e
// devolve um mapa resolução(string)→URL pública. Os thumbnails são gerados na
// transcodificação (um por resolução) em
// <MEDIA_DIR>/<tag>/<video_id>/thumb_<res>.jpg. Derivar do disco — em vez de
// manter um flag no banco — mantém o status sempre coerente com o que a rota
// pública realmente serve, e evita uma coluna nova (e o risco de esquecê-la
// em algum SELECT, como ocorreu na T53). Sempre devolve um mapa não-nil para
// que o JSON traga {} em vez de null.
func (h *StatusHandler) collectThumbnails(r *http.Request, video *models.Video) map[string]string {
	thumbnails := make(map[string]string)
	for _, res := range video.Resolutions {
		path := filepath.Join(h.cfg.MediaDir, models.Slugify(video.Tag), video.VideoID, models.ThumbnailFileName(res))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			thumbnails[strconv.Itoa(res)] = httputil.PublicThumbnailURL(r, video.Tag, video.VideoID, res)
		}
	}
	return thumbnails
}
