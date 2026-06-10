package serve

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/httputil"
	"github.com/klawdyo/streamedia/internal/models"
)

// PlayInitHandler trata POST /api/play/init: o backend principal, já tendo
// autorizado o usuário, troca o ROOT_TOKEN (validado pelo middleware RootAuth)
// por uma URL de reprodução assinada e de curta duração. Espelha o fluxo de
// /api/upload/init, do lado da leitura.
type PlayInitHandler struct {
	cfg *config.Config
	db  *sql.DB
}

// NewPlayInitHandler cria um PlayInitHandler com a config e o banco informados.
func NewPlayInitHandler(cfg *config.Config, db *sql.DB) *PlayInitHandler {
	return &PlayInitHandler{cfg: cfg, db: db}
}

// playInitRequest é o corpo esperado: o vídeo para o qual emitir a URL.
type playInitRequest struct {
	VideoID string `json:"video_id"`
}

// ServeHTTP valida o vídeo (existe + ready), gera um token de play efêmero e
// devolve a URL assinada do master playlist.
func (h *PlayInitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}
	var req playInitRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "JSON inválido.")
		return
	}
	if !models.IsValidVideoIDFormat(req.VideoID) {
		apiresponse.Error(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	video, err := models.GetVideo(h.db, req.VideoID)
	if err == sql.ErrNoRows {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao consultar o vídeo.")
		return
	}
	if video.Status != models.StatusReady {
		apiresponse.Error(w, http.StatusConflict, "Vídeo não está disponível para reprodução.")
		return
	}

	// Gera e persiste o token de play (string aleatória, purpose=play).
	token, err := auth.GenerateToken()
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao gerar o token de reprodução.")
		return
	}
	expiresAt := time.Now().Add(h.cfg.PlayTokenTTL)
	if err := models.InsertAccessToken(h.db, token, video.VideoID, models.PurposePlay, expiresAt); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o token de reprodução.")
		return
	}

	playURL := httputil.PublicPlayURL(r, video.Tag, video.VideoID, token)

	// Lista as resoluções disponíveis (variantes HLS geradas na transcodificação,
	// tabela video_renditions, ordenadas ASC). É informação auxiliar: se a
	// consulta falhar, ainda emitimos a play_url — o essencial para reprodução —
	// com a lista vazia, apenas logando o erro.
	resolutions := []int{}
	renditions, err := models.StorageByVideo(h.db, video.VideoID)
	if err != nil {
		log.Printf("[play] init: erro ao listar resoluções do vídeo %s: %v", video.VideoID, err)
	} else {
		for _, rd := range renditions {
			resolutions = append(resolutions, rd.Resolution)
		}
	}

	apiresponse.Success(w, http.StatusOK, map[string]any{
		"video_id":    video.VideoID,
		"tag":         video.Tag,
		"play_url":    playURL,
		"token":       token,
		"expires_at":  expiresAt.UTC().Format(time.RFC3339),
		"resolutions": resolutions,
	})
}
