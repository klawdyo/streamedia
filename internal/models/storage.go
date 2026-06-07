package models

import (
	"database/sql"
	"fmt"
)

// VideoRendition representa uma linha da tabela video_renditions: uma
// variante HLS gerada para um vídeo (issue #5, T36) — combinação única de
// (video_id, resolution), com o tamanho somado de seus segmentos .ts e a
// contagem deles. Preenchida pelo worker FFmpeg (T11) ao concluir cada
// variante.
//
// Decisão de granularidade (ver internal/db/schema.go): por vídeo + por
// variante de resolução, não por chunk de upload do protocolo TUS — os
// chunks são efêmeros (descartados após a montagem do arquivo final em
// UploadTmpDir, ver T07/T09) e não têm valor analítico duradouro. Esta
// granularidade atende ao pedido da issue #5 ("uma linha por arquivo
// salvo, com índices e valores corretos") sem reter dados temporários.
type VideoRendition struct {
	VideoID      string
	Resolution   int
	SizeBytes    int64
	SegmentCount int
}

// UpsertVideoRendition grava (ou substitui, em caso de re-transcodificação)
// o registro de uma variante HLS gerada — chave (video_id, resolution).
// Usa "INSERT ... ON CONFLICT ... DO UPDATE" para que reprocessar um vídeo
// não duplique linhas; sempre reflete o estado mais recente em disco.
func UpsertVideoRendition(db *sql.DB, videoID string, resolution int, sizeBytes int64, segmentCount int) error {
	_, err := db.Exec(
		`INSERT INTO video_renditions (video_id, resolution, size_bytes, segment_count, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT (video_id, resolution) DO UPDATE SET
		   size_bytes    = excluded.size_bytes,
		   segment_count = excluded.segment_count,
		   updated_at    = CURRENT_TIMESTAMP`,
		videoID, resolution, sizeBytes, segmentCount,
	)
	if err != nil {
		return fmt.Errorf("erro ao gravar variante (video_id=%s, resolution=%d): %w", videoID, resolution, err)
	}
	return nil
}

// StorageByVideo lista as variantes HLS geradas para um vídeo, ordenadas
// pela resolução — a "ficha de armazenamento" de um único vídeo.
func StorageByVideo(db *sql.DB, videoID string) ([]*VideoRendition, error) {
	rows, err := db.Query(
		`SELECT video_id, resolution, size_bytes, segment_count
		   FROM video_renditions WHERE video_id = ? ORDER BY resolution ASC`,
		videoID,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar variantes do vídeo %s: %w", videoID, err)
	}
	defer rows.Close()

	var renditions []*VideoRendition
	for rows.Next() {
		var r VideoRendition
		if err := rows.Scan(&r.VideoID, &r.Resolution, &r.SizeBytes, &r.SegmentCount); err != nil {
			return nil, fmt.Errorf("erro ao ler variante: %w", err)
		}
		renditions = append(renditions, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar variantes do vídeo %s: %w", videoID, err)
	}
	if renditions == nil {
		renditions = []*VideoRendition{}
	}
	return renditions, nil
}

// TotalStorageBytes soma o espaço total ocupado em disco: o tamanho dos
// arquivos originais (videos.actual_size_bytes — preenchido em T09 na
// validação pós-upload) mais o tamanho de todas as variantes HLS geradas
// (video_renditions.size_bytes — preenchido pelo worker ao final de cada
// variante). Responde a "quantos MB estão armazenados ao todo" (issue #5).
func TotalStorageBytes(db *sql.DB) (int64, error) {
	var originals, renditions sql.NullInt64
	if err := db.QueryRow(`SELECT COALESCE(SUM(actual_size_bytes), 0) FROM videos`).Scan(&originals); err != nil {
		return 0, fmt.Errorf("erro ao somar o tamanho dos arquivos originais: %w", err)
	}
	if err := db.QueryRow(`SELECT COALESCE(SUM(size_bytes), 0) FROM video_renditions`).Scan(&renditions); err != nil {
		return 0, fmt.Errorf("erro ao somar o tamanho das variantes: %w", err)
	}
	return originals.Int64 + renditions.Int64, nil
}

// TotalDurationSeconds soma a duração (em segundos) de todos os vídeos —
// usada para responder "quantos minutos de vídeo estão armazenados ao
// todo" (issue #5). Cada vídeo conta uma única vez (duration_s é a duração
// do conteúdo original; as variantes compartilham a mesma duração).
func TotalDurationSeconds(db *sql.DB) (int64, error) {
	var total sql.NullInt64
	if err := db.QueryRow(`SELECT COALESCE(SUM(duration_s), 0) FROM videos`).Scan(&total); err != nil {
		return 0, fmt.Errorf("erro ao somar a duração dos vídeos: %w", err)
	}
	return total.Int64, nil
}

// CountVideosByStatus agrupa a contagem de vídeos por status — responde
// "quantos arquivos estão pendentes/em processamento/prontos/com falha"
// (issue #5). Estados sem nenhum vídeo simplesmente não aparecem no mapa.
func CountVideosByStatus(db *sql.DB) (map[VideoStatus]int, error) {
	rows, err := db.Query(`SELECT status, COUNT(*) FROM videos GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("erro ao agrupar vídeos por status: %w", err)
	}
	defer rows.Close()

	counts := make(map[VideoStatus]int)
	for rows.Next() {
		var status VideoStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("erro ao ler contagem por status: %w", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar contagens por status: %w", err)
	}
	return counts, nil
}
