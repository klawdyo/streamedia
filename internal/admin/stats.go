package admin

import (
	"database/sql"
	"net/http"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// statsResponse é a estrutura de resposta da rota de estatísticas agregadas.
// VideoID é nil quando a agregação é global (sem filtro por vídeo).
type statsResponse struct {
	VideoID      *string          `json:"video_id"`
	Totals       map[string]int64 `json:"totals"`
	ByResolution map[int]int64    `json:"by_resolution"`
	ByOS         map[string]int64 `json:"by_os"`
	ByDayOfWeek  map[int]int64    `json:"by_day_of_week"`
	Storage      *storageStats    `json:"storage,omitempty"`
}

// storageStats agrega as estatísticas globais de armazenamento e fila
// (T36/T37, issue #5): espaço total ocupado, minutos de vídeo armazenados,
// contagem de vídeos por status e tamanho atual da fila de transcodificação.
type storageStats struct {
	TotalBytes           int64                      `json:"total_bytes"`
	TotalDurationSeconds int64                      `json:"total_duration_seconds"`
	VideosByStatus       map[models.VideoStatus]int `json:"videos_by_status"`
	QueuePending         int                        `json:"queue_pending"`
}

// eventTypes são os tipos de evento conhecidos (T26), usados para montar o
// mapa "totals" da resposta de forma determinística.
var eventTypes = []string{"playback", "download_segment", "upload_complete"}

// HandleStats retorna estatísticas agregadas de uso (T28), derivadas da
// tabela bruta playback_events (T26/T27): totais por tipo de evento,
// contagem por resolução, por família de SO e por dia da semana.
//
// Query params:
//   - video_id (opcional): restringe as agregações a um único vídeo.
//     Se informado e o vídeo não existir, retorna 404.
//
// Sem video_id, as agregações cobrem todos os vídeos (visão global).
func (h *AdminHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	videoID := r.URL.Query().Get("video_id")

	var videoIDPtr *string
	if videoID != "" {
		// Confirma que o vídeo existe antes de agregar — evita devolver
		// estatísticas vazias para um video_id inexistente (ambíguo com
		// "vídeo existe mas sem eventos ainda").
		if !respondIfVideoMissing(w, h.db, videoID) {
			return
		}
		videoIDPtr = &videoID
	}

	totals := make(map[string]int64, len(eventTypes))
	for _, eventType := range eventTypes {
		count, err := models.CountEventsByType(h.db, eventType, videoID)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas por tipo de evento.")
			return
		}
		totals[eventType] = count
	}

	byResolution, err := models.AggregateByResolution(h.db, videoID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas por resolução.")
		return
	}

	byOS, err := models.AggregateByOS(h.db, videoID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas por sistema operacional.")
		return
	}

	byDayOfWeek, err := models.AggregateByDayOfWeek(h.db, videoID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas por dia da semana.")
		return
	}

	resp := statsResponse{
		VideoID:      videoIDPtr,
		Totals:       totals,
		ByResolution: byResolution,
		ByOS:         byOS,
		ByDayOfWeek:  byDayOfWeek,
	}

	// A seção "storage" é uma visão agregada GLOBAL (espaço total, fila,
	// contagem por status — não faz sentido "por vídeo"). Por isso, quando
	// o filtro ?video_id= está presente, ela é omitida (omitempty) em vez
	// de devolver os mesmos totais globais disfarçados de "filtrados": isso
	// evitaria ambiguidade (o cliente poderia supor, erroneamente, que
	// total_bytes/queue_pending refletem apenas aquele vídeo).
	if videoIDPtr == nil {
		storage, err := buildStorageStats(h.db, h.queue)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas de armazenamento.")
			return
		}
		resp.Storage = storage
	}

	apiresponse.Success(w, http.StatusOK, resp)
}

// buildStorageStats monta a seção "storage" da resposta de /admin/stats
// (T36/T37, issue #5), reaproveitando as agregações de internal/models/storage.go
// (espaço total e duração, somando originais + variantes HLS) e a contagem de
// vídeos por status. queue_pending reaproveita a mesma fonte (queue.Len())
// usada por /admin/queue e pelo gauge streamedia_transcode_queue_length (T29)
// — não recomputa a fila por outro caminho.
func buildStorageStats(db *sql.DB, queue interface{ Len() int }) (*storageStats, error) {
	totalBytes, err := models.TotalStorageBytes(db)
	if err != nil {
		return nil, err
	}

	totalDuration, err := models.TotalDurationSeconds(db)
	if err != nil {
		return nil, err
	}

	videosByStatus, err := models.CountVideosByStatus(db)
	if err != nil {
		return nil, err
	}

	return &storageStats{
		TotalBytes:           totalBytes,
		TotalDurationSeconds: totalDuration,
		VideosByStatus:       videosByStatus,
		QueuePending:         queue.Len(),
	}, nil
}

// respondIfVideoMissing verifica se o vídeo existe. Caso não exista (ou
// ocorra erro de banco), já escreve a resposta de erro apropriada (404 ou
// 500) e retorna false — o chamador deve interromper o fluxo. Retorna true
// se o vídeo existe e o fluxo pode prosseguir normalmente.
func respondIfVideoMissing(w http.ResponseWriter, db *sql.DB, videoID string) bool {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM videos WHERE video_id = ?`, videoID).Scan(&exists)
	if err == sql.ErrNoRows {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não encontrado.")
		return false
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao consultar o vídeo.")
		return false
	}
	return true
}
