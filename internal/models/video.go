// Pacote models define as structs e queries de acesso ao banco.
package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// VideoStatus representa o estado de um vídeo na máquina de estados.
type VideoStatus string

// Estados possíveis de um vídeo ao longo do pipeline de upload e transcodificação.
const (
	StatusPendingUpload   VideoStatus = "pending_upload"
	StatusUploading       VideoStatus = "uploading"
	StatusUploadComplete  VideoStatus = "upload_complete"
	StatusTranscoding     VideoStatus = "transcoding"
	StatusReady           VideoStatus = "ready"
	StatusFailedUpload    VideoStatus = "failed_upload"
	StatusFailedTranscode VideoStatus = "failed_transcode"
)

// Video representa um registro da tabela videos.
type Video struct {
	VideoID           string
	Status            VideoStatus
	DeclaredSizeBytes int64
	ActualSizeBytes   int64
	DurationS         int
	Resolutions       []int // armazenado como JSON no banco
	TranscodeAttempts int
	LastChunkAt       *time.Time
	ErrorMessage      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// validTransitions define as transições permitidas por estado.
// Estados terminais (failed_upload, failed_transcode) e ready não aparecem como chave
// pois não têm transições de saída válidas.
var validTransitions = map[VideoStatus][]VideoStatus{
	StatusPendingUpload:  {StatusUploading, StatusFailedUpload},
	StatusUploading:      {StatusUploading, StatusUploadComplete, StatusFailedUpload},
	StatusUploadComplete: {StatusTranscoding},
	StatusTranscoding:    {StatusTranscoding, StatusReady, StatusFailedTranscode},
}

// isValidTransition retorna true se a transição do estado from para o estado to é permitida.
func isValidTransition(from, to VideoStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false // estado terminal ou desconhecido
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// InsertVideo cria um novo registro de vídeo com o status inicial pending_upload.
func InsertVideo(db *sql.DB, videoID string, declaredSize int64) error {
	_, err := db.Exec(
		"INSERT INTO videos (video_id, declared_size_bytes) VALUES (?, ?)",
		videoID, declaredSize,
	)
	if err != nil {
		return fmt.Errorf("erro ao inserir vídeo: %w", err)
	}
	return nil
}

// GetVideo busca um vídeo pelo seu ID e retorna a struct preenchida.
// Trata corretamente os campos que podem ser NULL no banco.
func GetVideo(db *sql.DB, videoID string) (*Video, error) {
	row := db.QueryRow(
		`SELECT video_id, status, declared_size_bytes, actual_size_bytes,
		        duration_s, resolutions, transcode_attempts, last_chunk_at,
		        error_message, created_at, updated_at
		   FROM videos WHERE video_id = ?`,
		videoID,
	)

	var (
		v            Video
		declaredSize sql.NullInt64
		actualSize   sql.NullInt64
		durationS    sql.NullInt64
		resolutions  sql.NullString
		lastChunkAt  sql.NullTime
		errorMessage sql.NullString
	)

	err := row.Scan(
		&v.VideoID,
		&v.Status,
		&declaredSize,
		&actualSize,
		&durationS,
		&resolutions,
		&v.TranscodeAttempts,
		&lastChunkAt,
		&errorMessage,
		&v.CreatedAt,
		&v.UpdatedAt,
	)
	if err != nil {
		// Repassa sql.ErrNoRows sem embrulhar para o chamador poder identificar
		return nil, err
	}

	// Converte os campos nulos para os tipos da struct
	v.DeclaredSizeBytes = declaredSize.Int64
	v.ActualSizeBytes = actualSize.Int64
	v.DurationS = int(durationS.Int64)
	v.ErrorMessage = errorMessage.String

	if lastChunkAt.Valid {
		t := lastChunkAt.Time
		v.LastChunkAt = &t
	}

	// Deserializa resolutions: NULL → []int{}, caso contrário faz unmarshal do JSON
	if resolutions.Valid && resolutions.String != "" {
		if err := json.Unmarshal([]byte(resolutions.String), &v.Resolutions); err != nil {
			return nil, fmt.Errorf("erro ao deserializar resolutions: %w", err)
		}
	} else {
		v.Resolutions = []int{}
	}

	return &v, nil
}

// UpdateStatus altera o status do vídeo, validando a transição na máquina de estados.
func UpdateStatus(db *sql.DB, videoID string, newStatus VideoStatus) error {
	// Busca o status atual para validar a transição
	current, err := GetVideo(db, videoID)
	if err != nil {
		return err
	}

	if !isValidTransition(current.Status, newStatus) {
		return fmt.Errorf("Transição de estado inválida: %s → %s", current.Status, newStatus)
	}

	_, err = db.Exec(
		"UPDATE videos SET status = ? WHERE video_id = ?",
		newStatus, videoID,
	)
	if err != nil {
		return fmt.Errorf("erro ao atualizar status: %w", err)
	}
	return nil
}

// UpdateStatusWithError altera o status validando a transição e também grava a mensagem de erro.
func UpdateStatusWithError(db *sql.DB, videoID string, newStatus VideoStatus, errMsg string) error {
	// Busca o status atual para validar a transição
	current, err := GetVideo(db, videoID)
	if err != nil {
		return err
	}

	if !isValidTransition(current.Status, newStatus) {
		return fmt.Errorf("Transição de estado inválida: %s → %s", current.Status, newStatus)
	}

	_, err = db.Exec(
		"UPDATE videos SET status = ?, error_message = ? WHERE video_id = ?",
		newStatus, errMsg, videoID,
	)
	if err != nil {
		return fmt.Errorf("erro ao atualizar status com erro: %w", err)
	}
	return nil
}

// UpdateLastChunk atualiza o timestamp do último chunk recebido para o momento atual.
func UpdateLastChunk(db *sql.DB, videoID string) error {
	_, err := db.Exec(
		"UPDATE videos SET last_chunk_at = CURRENT_TIMESTAMP WHERE video_id = ?",
		videoID,
	)
	if err != nil {
		return fmt.Errorf("erro ao atualizar last_chunk_at: %w", err)
	}
	return nil
}

// SetUploadComplete transiciona o vídeo para upload_complete e grava o tamanho real recebido.
func SetUploadComplete(db *sql.DB, videoID string, actualSize int64) error {
	// Valida e aplica a transição de estado
	if err := UpdateStatus(db, videoID, StatusUploadComplete); err != nil {
		return err
	}

	_, err := db.Exec(
		"UPDATE videos SET actual_size_bytes = ? WHERE video_id = ?",
		actualSize, videoID,
	)
	if err != nil {
		return fmt.Errorf("erro ao atualizar actual_size_bytes: %w", err)
	}
	return nil
}

// SetReady transiciona o vídeo para ready e grava duração e resolutions (serializadas como JSON).
func SetReady(db *sql.DB, videoID string, durationS int, resolutions []int) error {
	// Valida e aplica a transição de estado
	if err := UpdateStatus(db, videoID, StatusReady); err != nil {
		return err
	}

	// Serializa as resolutions como JSON para armazenar como texto
	data, err := json.Marshal(resolutions)
	if err != nil {
		return fmt.Errorf("erro ao serializar resolutions: %w", err)
	}

	_, err = db.Exec(
		"UPDATE videos SET duration_s = ?, resolutions = ? WHERE video_id = ?",
		durationS, string(data), videoID,
	)
	if err != nil {
		return fmt.Errorf("erro ao atualizar duration_s e resolutions: %w", err)
	}
	return nil
}

// IncrementTranscodeAttempts incrementa em 1 o contador de tentativas de transcodificação.
func IncrementTranscodeAttempts(db *sql.DB, videoID string) error {
	_, err := db.Exec(
		"UPDATE videos SET transcode_attempts = transcode_attempts + 1 WHERE video_id = ?",
		videoID,
	)
	if err != nil {
		return fmt.Errorf("erro ao incrementar transcode_attempts: %w", err)
	}
	return nil
}

// ListByStatus retorna todos os vídeos que estão no status informado.
func ListByStatus(db *sql.DB, status VideoStatus) ([]*Video, error) {
	rows, err := db.Query(
		`SELECT video_id, status, declared_size_bytes, actual_size_bytes,
		        duration_s, resolutions, transcode_attempts, last_chunk_at,
		        error_message, created_at, updated_at
		   FROM videos WHERE status = ?`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar vídeos por status: %w", err)
	}
	defer rows.Close()

	var videos []*Video
	for rows.Next() {
		var (
			v            Video
			declaredSize sql.NullInt64
			actualSize   sql.NullInt64
			durationS    sql.NullInt64
			resolutions  sql.NullString
			lastChunkAt  sql.NullTime
			errorMessage sql.NullString
		)

		err := rows.Scan(
			&v.VideoID,
			&v.Status,
			&declaredSize,
			&actualSize,
			&durationS,
			&resolutions,
			&v.TranscodeAttempts,
			&lastChunkAt,
			&errorMessage,
			&v.CreatedAt,
			&v.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler linha de vídeo: %w", err)
		}

		v.DeclaredSizeBytes = declaredSize.Int64
		v.ActualSizeBytes = actualSize.Int64
		v.DurationS = int(durationS.Int64)
		v.ErrorMessage = errorMessage.String

		if lastChunkAt.Valid {
			t := lastChunkAt.Time
			v.LastChunkAt = &t
		}

		if resolutions.Valid && resolutions.String != "" {
			if err := json.Unmarshal([]byte(resolutions.String), &v.Resolutions); err != nil {
				return nil, fmt.Errorf("erro ao deserializar resolutions: %w", err)
			}
		} else {
			v.Resolutions = []int{}
		}

		videos = append(videos, &v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar vídeos: %w", err)
	}

	return videos, nil
}
