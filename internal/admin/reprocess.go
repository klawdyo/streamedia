// Pacote admin — handler de reprocessamento de vídeo.
//
// Permite que qualquer usuário autenticado reenvie um vídeo para a fila de
// transcodificação, independente do estado atual. Útil para recuperar vídeos
// que falharam na transcodificação ou para regerar segmentos HLS após mudança
// nas configurações de qualidade.
package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// HandleReprocessVideo reenfileira um vídeo para transcodificação.
// O vídeo é colocado no estado "transcoding" e seu contador de tentativas
// é resetado para 0 — como se fosse um vídeo recém-enviado.
//
// POST /api/videos/{videoID}/reprocess
//
// Só pode ser chamado por usuários autenticados (qualquer role). O ROOT_TOKEN
// também funciona (acesso backend-to-backend).
//
// Estados de origem permitidos: ready, failed_transcode, failed_upload.
// Vídeos em transcode ativo ou upload em andamento não podem ser reprocessados.
func (h *AdminHandler) HandleReprocessVideo(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "videoID")
	if videoID == "" {
		apiresponse.Error(w, http.StatusBadRequest, "ID do vídeo é obrigatório.")
		return
	}

	// Busca o vídeo atual.
	video, err := models.GetVideo(h.db, videoID)
	if err != nil {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}

	// Só permite reprocessar vídeos que estão em estados terminais ou estáveis.
	// Vídeos que ainda estão em pipeline (pending_upload, uploading, upload_complete,
	// transcoding) não devem ser reprocessados — isso criaria condições de corrida
	// com o worker ou o TUS handler.
	switch video.Status {
	case models.StatusReady, models.StatusFailedTranscode, models.StatusFailedUpload:
		// Permitido.
	default:
		apiresponse.Error(w, http.StatusConflict,
			"Este vídeo está em um estado que não permite reprocessamento. Aguarde a conclusão do pipeline atual.")
		return
	}

	// Reseta o contador de tentativas e coloca o vídeo como transcoding.
	// O job de requeue (se configurado) ou o worker normal vão pegá-lo.
	if err := models.UpdateStatus(h.db, videoID, models.StatusTranscoding); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao reprocessar o vídeo.")
		return
	}

	// Reseta as tentativas de transcodificação.
	if _, err := h.db.Exec(`UPDATE videos SET transcode_attempts = 0 WHERE video_id = ?`, videoID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao resetar tentativas de transcodificação.")
		return
	}

	apiresponse.Success(w, http.StatusOK, map[string]string{
		"video_id": videoID,
		"status":   string(models.StatusTranscoding),
		"message":  "Vídeo enfileirado para reprocessamento.",
	})
}
