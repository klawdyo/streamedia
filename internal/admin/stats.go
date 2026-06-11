package admin

import (
	"database/sql"
	"net/http"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// statsResponse é a estrutura de resposta da rota de estatísticas agregadas.
// VideoID é nil quando a agregação é global (sem filtro por vídeo).
//
// As agregações de PLAYBACK (totais por tipo, por resolução, por SO, por
// dia-da-semana, por hora e por data) respeitam o filtro ?video_id=; as
// seções globais (storage e uploads) só aparecem na visão global — ver o
// comentário em HandleStats.
type statsResponse struct {
	VideoID      *string          `json:"video_id"`
	Totals       map[string]int64 `json:"totals"`
	ByResolution map[int]int64    `json:"by_resolution"`
	ByOS         map[string]int64 `json:"by_os"`
	ByDayOfWeek  map[int]int64    `json:"by_day_of_week"`
	// ByHour e ByDate (dashboard): distribuição das reproduções por hora do
	// dia (0..23) e por data (YYYY-MM-DD) — alimentam os gráficos de
	// "horários/dias mais movimentados".
	ByHour map[int]int64    `json:"by_hour"`
	ByDate map[string]int64 `json:"by_date"`
	// Storage e Uploads são visões GLOBAIS (omitidas quando ?video_id= está
	// presente). Uploads agrega a movimentação de envios (videos.created_at).
	Storage *storageStats `json:"storage,omitempty"`
	Uploads *uploadsStats `json:"uploads,omitempty"`
	// VideoStorage é a ficha de armazenamento de UM vídeo (renditions + totais),
	// presente apenas quando ?video_id= aponta um vídeo existente.
	VideoStorage *videoStorageStats `json:"video_storage,omitempty"`
}

// storageStats agrega as estatísticas globais de armazenamento e fila
// (T36/T37, issue #5): espaço total ocupado, minutos de vídeo armazenados,
// contagem de vídeos por status, tamanho atual da fila de transcodificação e
// o número de workers de transcodificação (para o dashboard montar o cartão
// de fila sem uma segunda chamada a /admin/queue).
type storageStats struct {
	TotalBytes           int64                      `json:"total_bytes"`
	TotalDurationSeconds int64                      `json:"total_duration_seconds"`
	VideosByStatus       map[models.VideoStatus]int `json:"videos_by_status"`
	QueuePending         int                        `json:"queue_pending"`
	Workers              int                        `json:"workers"`
}

// uploadsStats agrega a movimentação de envios (tabela videos): total e
// distribuição por data, por dia-da-semana (0=domingo..6=sábado) e por hora
// (0..23) — alimenta os gráficos de uploads do dashboard.
type uploadsStats struct {
	Total       int64            `json:"total"`
	ByDate      map[string]int64 `json:"by_date"`
	ByDayOfWeek map[int]int64    `json:"by_day_of_week"`
	ByHour      map[int]int64    `json:"by_hour"`
}

// videoStorageStats é a ficha de armazenamento de um único vídeo: suas
// variantes HLS (renditions), o total de bytes ocupado e a duração. Reúne o
// que a página de vídeo do dashboard precisa para mostrar "peso por resolução".
type videoStorageStats struct {
	Renditions      []renditionStat `json:"renditions"`
	TotalBytes      int64           `json:"total_bytes"`
	DurationSeconds int             `json:"duration_seconds"`
}

// renditionStat é a projeção pública de uma variante HLS (sem repetir o
// video_id, já conhecido pelo cliente). Espelha models.VideoRendition.
type renditionStat struct {
	Resolution   int   `json:"resolution"`
	SizeBytes    int64 `json:"size_bytes"`
	SegmentCount int   `json:"segment_count"`
}

// eventTypes são os tipos de evento conhecidos (T26), usados para montar o
// mapa "totals" da resposta de forma determinística.
var eventTypes = []string{"playback", "download_segment", "upload_complete"}

// HandleStats retorna estatísticas agregadas de uso (T28), derivadas da
// tabela bruta playback_events (T26/T27): totais por tipo de evento,
// contagem por resolução, por família de SO, por dia da semana, por hora e
// por data.
//
// Query params:
//   - video_id (opcional): restringe as agregações de PLAYBACK a um único
//     vídeo. Se informado e o vídeo não existir, retorna 404.
//
// Sem video_id, as agregações cobrem todos os vídeos (visão global) e a
// resposta inclui as seções globais "storage" (espaço/fila) e "uploads"
// (movimentação de envios). Com video_id, a resposta inclui "video_storage"
// (ficha de armazenamento daquele vídeo).
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

	byHour, err := models.PlaybackByHour(h.db, videoID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas por hora.")
		return
	}

	byDate, err := models.PlaybackByDate(h.db, videoID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas por data.")
		return
	}

	resp := statsResponse{
		VideoID:      videoIDPtr,
		Totals:       totals,
		ByResolution: byResolution,
		ByOS:         byOS,
		ByDayOfWeek:  byDayOfWeek,
		ByHour:       byHour,
		ByDate:       byDate,
	}

	if videoIDPtr == nil {
		// Visão GLOBAL: storage (espaço total, fila, contagem por status) e
		// uploads (movimentação de envios). Não fazem sentido "por vídeo" — por
		// isso são omitidas (omitempty) quando ?video_id= está presente, em vez
		// de devolver os mesmos totais globais disfarçados de "filtrados".
		storage, err := buildStorageStats(h.db, h.queue, h.cfg.TranscodeWorkers)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas de armazenamento.")
			return
		}
		resp.Storage = storage

		uploads, err := buildUploadsStats(h.db)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar estatísticas de uploads.")
			return
		}
		resp.Uploads = uploads
	} else {
		// Visão POR VÍDEO: a ficha de armazenamento daquele vídeo (suas
		// variantes HLS e o peso de cada uma) é informação legitimamente
		// per-vídeo — distinta do storage global.
		videoStorage, err := buildVideoStorageStats(h.db, videoID)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao agregar o armazenamento do vídeo.")
			return
		}
		resp.VideoStorage = videoStorage
	}

	apiresponse.Success(w, http.StatusOK, resp)
}

// buildStorageStats monta a seção "storage" da resposta de /admin/stats
// (T36/T37, issue #5), reaproveitando as agregações de internal/models/storage.go
// (espaço total e duração, somando originais + variantes HLS) e a contagem de
// vídeos por status. queue_pending reaproveita a mesma fonte (queue.Len())
// usada por /admin/queue e pelo gauge streamedia_transcode_queue_length (T29)
// — não recomputa a fila por outro caminho. workers é o nº de workers
// configurado (mesma fonte de /admin/queue).
func buildStorageStats(db *sql.DB, queue interface{ Len() int }, workers int) (*storageStats, error) {
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
		Workers:              workers,
	}, nil
}

// buildUploadsStats monta a seção "uploads" da resposta global de /admin/stats,
// reaproveitando as agregações de internal/models/timeseries.go (total e
// distribuição de envios por data/dia-da-semana/hora a partir de
// videos.created_at).
func buildUploadsStats(db *sql.DB) (*uploadsStats, error) {
	total, err := models.CountVideos(db)
	if err != nil {
		return nil, err
	}
	byDate, err := models.UploadsByDate(db)
	if err != nil {
		return nil, err
	}
	byDayOfWeek, err := models.UploadsByDayOfWeek(db)
	if err != nil {
		return nil, err
	}
	byHour, err := models.UploadsByHour(db)
	if err != nil {
		return nil, err
	}
	return &uploadsStats{
		Total:       total,
		ByDate:      byDate,
		ByDayOfWeek: byDayOfWeek,
		ByHour:      byHour,
	}, nil
}

// buildVideoStorageStats monta a ficha de armazenamento de um vídeo:
// reaproveita models.StorageByVideo (renditions) e models.GetVideo (duração),
// e soma o total de bytes das variantes. Projeta cada VideoRendition em
// renditionStat (sem repetir o video_id).
func buildVideoStorageStats(db *sql.DB, videoID string) (*videoStorageStats, error) {
	renditions, err := models.StorageByVideo(db, videoID)
	if err != nil {
		return nil, err
	}

	video, err := models.GetVideo(db, videoID)
	if err != nil {
		return nil, err
	}

	out := &videoStorageStats{
		Renditions:      make([]renditionStat, 0, len(renditions)),
		DurationSeconds: video.DurationS,
	}
	for _, rnd := range renditions {
		out.Renditions = append(out.Renditions, renditionStat{
			Resolution:   rnd.Resolution,
			SizeBytes:    rnd.SizeBytes,
			SegmentCount: rnd.SegmentCount,
		})
		out.TotalBytes += rnd.SizeBytes
	}
	return out, nil
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
