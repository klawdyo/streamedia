package models

import (
	"database/sql"
	"strings"
)

// RecordEvent insere um evento de uso (playback, download de segmento ou
// upload concluído) na tabela playback_events. resolution pode ser nil
// para eventos sem resolução associada (ex. master.m3u8, upload_complete).
//
// O os_family é derivado automaticamente do user-agent via detectOSFamily.
func RecordEvent(db *sql.DB, videoID, eventType string, resolution *int, userAgent string) error {
	osFamily := detectOSFamily(userAgent)
	_, err := db.Exec(
		`INSERT INTO playback_events (video_id, event_type, resolution, user_agent, os_family)
		 VALUES (?, ?, ?, ?, ?)`,
		videoID, eventType, resolution, userAgent, osFamily,
	)
	return err
}

// detectOSFamily classifica o user-agent em uma família de SO conhecida,
// usando parsing simples por substring (sem dependência externa).
//
// Ordem de prioridade: iOS (iPhone/iPad/iPod) antes de macOS, pois o
// user-agent de dispositivos iOS também contém "Mac OS" em alguns casos;
// Android antes de Linux, pois o user-agent Android também contém "Linux".
func detectOSFamily(userAgent string) string {
	ua := strings.ToLower(userAgent)

	switch {
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"), strings.Contains(ua, "ipod"):
		return "ios"
	case strings.Contains(ua, "android"):
		return "android"
	case strings.Contains(ua, "windows"):
		return "windows"
	case strings.Contains(ua, "macintosh"), strings.Contains(ua, "mac os"):
		return "macos"
	case strings.Contains(ua, "linux"):
		return "linux"
	default:
		return "other"
	}
}

// CountEventsByType retorna o total de eventos registrados de um tipo
// específico (ex. "playback", "download_segment", "upload_complete").
//
// Se videoID for não-vazio, restringe a contagem àquele vídeo; caso
// contrário, conta em todos os vídeos (usado pela agregação global de T28).
func CountEventsByType(db *sql.DB, eventType string, videoID ...string) (int64, error) {
	var count int64
	var err error
	if len(videoID) > 0 && videoID[0] != "" {
		err = db.QueryRow(
			`SELECT COUNT(*) FROM playback_events WHERE event_type = ? AND video_id = ?`,
			eventType, videoID[0],
		).Scan(&count)
	} else {
		err = db.QueryRow(
			`SELECT COUNT(*) FROM playback_events WHERE event_type = ?`,
			eventType,
		).Scan(&count)
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}

// AggregateByResolution retorna a contagem de eventos por resolução.
// Eventos com resolution NULL são ignorados (não fazem sentido em uma
// agregação por resolução).
//
// Se videoID for não-vazio, restringe a agregação àquele vídeo; caso
// contrário, agrega em todos os vídeos (visão global usada por T28).
func AggregateByResolution(db *sql.DB, videoID string) (map[int]int64, error) {
	var rows *sql.Rows
	var err error
	if videoID != "" {
		rows, err = db.Query(
			`SELECT resolution, COUNT(*) FROM playback_events
			 WHERE video_id = ? AND resolution IS NOT NULL
			 GROUP BY resolution`,
			videoID,
		)
	} else {
		rows, err = db.Query(
			`SELECT resolution, COUNT(*) FROM playback_events
			 WHERE resolution IS NOT NULL
			 GROUP BY resolution`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]int64)
	for rows.Next() {
		var resolution int
		var count int64
		if err := rows.Scan(&resolution, &count); err != nil {
			return nil, err
		}
		result[resolution] = count
	}
	return result, rows.Err()
}

// AggregateByOS retorna a contagem de eventos por família de SO.
//
// Se videoID for não-vazio, restringe a agregação àquele vídeo; caso
// contrário, agrega em todos os vídeos (usado pela agregação global de T28).
func AggregateByOS(db *sql.DB, videoID ...string) (map[string]int64, error) {
	var rows *sql.Rows
	var err error
	if len(videoID) > 0 && videoID[0] != "" {
		rows, err = db.Query(
			`SELECT os_family, COUNT(*) FROM playback_events
			 WHERE video_id = ? GROUP BY os_family`,
			videoID[0],
		)
	} else {
		rows, err = db.Query(
			`SELECT os_family, COUNT(*) FROM playback_events
			 GROUP BY os_family`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var osFamily string
		var count int64
		if err := rows.Scan(&osFamily, &count); err != nil {
			return nil, err
		}
		result[osFamily] = count
	}
	return result, rows.Err()
}

// AggregateByDayOfWeek retorna a contagem de eventos por dia da semana
// (0=domingo .. 6=sábado), derivado de occurred_at via strftime('%w', ...).
//
// Normalizamos com datetime() antes de aplicar strftime() para evitar o
// bug de comparação/formatação de datas (RFC3339 com "T" vs formato SQLite
// com espaço) já corrigido em T14/T16.
//
// Se videoID for não-vazio, restringe a agregação àquele vídeo; caso
// contrário, agrega em todos os vídeos (usado pela agregação global de T28).
func AggregateByDayOfWeek(db *sql.DB, videoID ...string) (map[int]int64, error) {
	var rows *sql.Rows
	var err error
	if len(videoID) > 0 && videoID[0] != "" {
		rows, err = db.Query(
			`SELECT CAST(strftime('%w', datetime(occurred_at)) AS INTEGER), COUNT(*)
			 FROM playback_events
			 WHERE video_id = ?
			 GROUP BY strftime('%w', datetime(occurred_at))`,
			videoID[0],
		)
	} else {
		rows, err = db.Query(
			`SELECT CAST(strftime('%w', datetime(occurred_at)) AS INTEGER), COUNT(*)
			 FROM playback_events
			 GROUP BY strftime('%w', datetime(occurred_at))`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]int64)
	for rows.Next() {
		var dayOfWeek int
		var count int64
		if err := rows.Scan(&dayOfWeek, &count); err != nil {
			return nil, err
		}
		result[dayOfWeek] = count
	}
	return result, rows.Err()
}
