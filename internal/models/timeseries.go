package models

import (
	"database/sql"
)

// Este arquivo agrega séries temporais para o dashboard administrativo:
// movimentação de UPLOADS (tabela videos, coluna created_at) e de
// REPRODUÇÕES (tabela playback_events, coluna occurred_at), agrupadas por
// data, dia-da-semana e hora — respondendo "quais dias e horários são mais
// movimentados".
//
// Como em internal/models/stats.go, normalizamos o timestamp com datetime()
// antes de aplicar strftime(): valores gravados em RFC3339 (com "T") e os
// gravados pelo SQLite (com espaço) precisam do mesmo formato para o strftime
// classificar corretamente o bucket — o mesmo cuidado já tomado em T14/T16.

// ---------------------------------------------------------------------------
// Uploads (tabela videos, created_at)
// ---------------------------------------------------------------------------

// UploadsByDate retorna a contagem de vídeos enviados por data (YYYY-MM-DD),
// derivada de videos.created_at. Útil para o gráfico de "dia com mais vídeos".
func UploadsByDate(db *sql.DB) (map[string]int64, error) {
	rows, err := db.Query(
		`SELECT strftime('%Y-%m-%d', datetime(created_at)) AS d, COUNT(*)
		 FROM videos
		 GROUP BY d
		 ORDER BY d`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var date string
		var count int64
		if err := rows.Scan(&date, &count); err != nil {
			return nil, err
		}
		result[date] = count
	}
	return result, rows.Err()
}

// UploadsByDayOfWeek retorna a contagem de vídeos enviados por dia da semana
// (0=domingo .. 6=sábado), derivada de videos.created_at.
func UploadsByDayOfWeek(db *sql.DB) (map[int]int64, error) {
	return scanIntCounts(db,
		`SELECT CAST(strftime('%w', datetime(created_at)) AS INTEGER), COUNT(*)
		 FROM videos
		 GROUP BY strftime('%w', datetime(created_at))`,
	)
}

// UploadsByHour retorna a contagem de vídeos enviados por hora do dia
// (0..23), derivada de videos.created_at.
func UploadsByHour(db *sql.DB) (map[int]int64, error) {
	return scanIntCounts(db,
		`SELECT CAST(strftime('%H', datetime(created_at)) AS INTEGER), COUNT(*)
		 FROM videos
		 GROUP BY strftime('%H', datetime(created_at))`,
	)
}

// CountVideos retorna o total de vídeos registrados (cartão de overview).
func CountVideos(db *sql.DB) (int64, error) {
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM videos`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// Reproduções (tabela playback_events, occurred_at)
// ---------------------------------------------------------------------------

// PlaybackByDate retorna a contagem de eventos de playback por data
// (YYYY-MM-DD). Se videoID for não-vazio, restringe ao vídeo; caso contrário
// agrega em todos os vídeos (mesma convenção das demais agregações de playback).
func PlaybackByDate(db *sql.DB, videoID ...string) (map[string]int64, error) {
	var rows *sql.Rows
	var err error
	if len(videoID) > 0 && videoID[0] != "" {
		rows, err = db.Query(
			`SELECT strftime('%Y-%m-%d', datetime(occurred_at)) AS d, COUNT(*)
			 FROM playback_events
			 WHERE video_id = ?
			 GROUP BY d
			 ORDER BY d`,
			videoID[0],
		)
	} else {
		rows, err = db.Query(
			`SELECT strftime('%Y-%m-%d', datetime(occurred_at)) AS d, COUNT(*)
			 FROM playback_events
			 GROUP BY d
			 ORDER BY d`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var date string
		var count int64
		if err := rows.Scan(&date, &count); err != nil {
			return nil, err
		}
		result[date] = count
	}
	return result, rows.Err()
}

// PlaybackByHour retorna a contagem de eventos de playback por hora do dia
// (0..23). Se videoID for não-vazio, restringe ao vídeo.
func PlaybackByHour(db *sql.DB, videoID ...string) (map[int]int64, error) {
	if len(videoID) > 0 && videoID[0] != "" {
		return scanIntCounts(db,
			`SELECT CAST(strftime('%H', datetime(occurred_at)) AS INTEGER), COUNT(*)
			 FROM playback_events
			 WHERE video_id = ?
			 GROUP BY strftime('%H', datetime(occurred_at))`,
			videoID[0],
		)
	}
	return scanIntCounts(db,
		`SELECT CAST(strftime('%H', datetime(occurred_at)) AS INTEGER), COUNT(*)
		 FROM playback_events
		 GROUP BY strftime('%H', datetime(occurred_at))`,
	)
}

// scanIntCounts executa uma query que devolve dois campos — uma chave inteira
// (bucket) e uma contagem — e materializa o mapa chave→contagem. Centraliza o
// laço de leitura comum às agregações por dia-da-semana e por hora.
func scanIntCounts(db *sql.DB, query string, args ...interface{}) (map[int]int64, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]int64)
	for rows.Next() {
		var bucket int
		var count int64
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		result[bucket] = count
	}
	return result, rows.Err()
}
