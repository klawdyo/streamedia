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
// Os json tags usam snake_case para compatibilidade com o frontend Vue.
type Video struct {
	VideoID           string      `json:"video_id"`
	Status            VideoStatus `json:"status"`
	DeclaredSizeBytes int64       `json:"declared_size_bytes"`
	ActualSizeBytes   int64       `json:"actual_size_bytes"`
	DurationS         int         `json:"duration_s"`
	Resolutions       []int       `json:"resolutions"` // armazenado como JSON no banco
	TranscodeAttempts int         `json:"transcode_attempts"`
	LastChunkAt       *time.Time  `json:"last_chunk_at,omitempty"`
	ErrorMessage      string      `json:"error_message,omitempty"`
	// Tag é o namespace (slug) do vídeo: define o diretório de armazenamento
	// (<MEDIA_DIR>/<tag>/<video_id>/...) e agrupa vídeos para consultas. Não
	// é credencial — toda autenticação é feita com o ROOT_TOKEN único.
	Tag string `json:"tag"`
	// WebhookURL é a URL de webhook customizada deste vídeo (issue #20).
	// Quando preenchida (HTTPS, informada no upload/init), os webhooks deste
	// vídeo vão para ela em vez da WEBHOOK_URL global. Vazia ("") = usar a
	// URL global (comportamento histórico).
	WebhookURL string    `json:"webhook_url,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// HasThumbnails indica se há thumbnails (posters) no disco para este vídeo.
	// Preenchido pelo handler da lista, não pelo banco — sempre false após ScanVideoRow.
	HasThumbnails bool `json:"has_thumbnails"`
	// ThumbnailURL é a URL pública do menor thumbnail disponível para preview na lista.
	// Preenchido pelo handler da lista, não pelo banco — sempre vazio após ScanVideoRow.
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

// validTransitions define as transições permitidas por estado.
// Estados terminais (failed_upload, failed_transcode) e ready não aparecem como chave
// pois não têm transições de saída válidas.
var validTransitions = map[VideoStatus][]VideoStatus{
	StatusPendingUpload:  {StatusUploading, StatusFailedUpload},
	StatusUploading:      {StatusUploading, StatusUploadComplete, StatusFailedUpload},
	StatusUploadComplete: {StatusTranscoding},
	StatusTranscoding:    {StatusTranscoding, StatusReady, StatusFailedTranscode, StatusUploadComplete},
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

// SelectVideoColumns é a lista canônica de colunas para SELECT de vídeos.
// Usada em GetVideo, ListByStatus e admin.HandleVideos para garantir que
// todas as queries retornam as mesmas colunas na mesma ordem.
const SelectVideoColumns = `video_id, status, declared_size_bytes, actual_size_bytes,
	duration_s, resolutions, transcode_attempts, last_chunk_at,
	error_message, tag, created_at, updated_at, webhook_url`

// ScanVideoRow lê uma linha de Video do banco, tratando campos nullable.
// Aceita qualquer função que implemente a assinatura de Scan (sql.Row e
// sql.Rows compartilham a mesma assinatura).
func ScanVideoRow(scan func(dest ...any) error) (*Video, error) {
	var (
		v            Video
		declaredSize sql.NullInt64
		actualSize   sql.NullInt64
		durationS    sql.NullInt64
		resolutions  sql.NullString
		lastChunkAt  sql.NullTime
		errorMessage sql.NullString
		webhookURL   sql.NullString
	)

	err := scan(
		&v.VideoID,
		&v.Status,
		&declaredSize,
		&actualSize,
		&durationS,
		&resolutions,
		&v.TranscodeAttempts,
		&lastChunkAt,
		&errorMessage,
		&v.Tag,
		&v.CreatedAt,
		&v.UpdatedAt,
		&webhookURL,
	)
	if err != nil {
		return nil, err
	}

	v.DeclaredSizeBytes = declaredSize.Int64
	v.ActualSizeBytes = actualSize.Int64
	v.DurationS = int(durationS.Int64)
	v.ErrorMessage = errorMessage.String
	v.WebhookURL = webhookURL.String

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

	return &v, nil
}

// InsertVideo cria um novo registro de vídeo com o status inicial
// pending_upload, no namespace (tag) padrão "default". Conveniência para
// chamadas que não se importam com a tag (ex.: testes). Para definir a tag,
// use InsertVideoWithTag.
func InsertVideo(db *sql.DB, videoID string, declaredSize int64) error {
	return InsertVideoWithTag(db, videoID, declaredSize, "default")
}

// InsertVideoWithTag cria um novo registro de vídeo no namespace (tag)
// informado. A tag deve vir já normalizada pelo chamador (models.Slugify).
// O webhook_url nasce vazio (usa a WEBHOOK_URL global); para definir uma URL
// customizada por vídeo (issue #20) use InsertVideoWithTagAndWebhook.
func InsertVideoWithTag(db *sql.DB, videoID string, declaredSize int64, tag string) error {
	return InsertVideoWithTagAndWebhook(db, videoID, declaredSize, tag, "")
}

// InsertVideoWithTagAndWebhook cria um novo registro de vídeo no namespace
// (tag) informado, com uma URL de webhook customizada (issue #20). A tag deve
// vir já normalizada pelo chamador (models.Slugify) e webhookURL já validada
// (HTTPS, formato e tamanho) — passe "" para usar a WEBHOOK_URL global.
func InsertVideoWithTagAndWebhook(db *sql.DB, videoID string, declaredSize int64, tag, webhookURL string) error {
	_, err := db.Exec(
		"INSERT INTO videos (video_id, declared_size_bytes, tag, webhook_url) VALUES (?, ?, ?, ?)",
		videoID, declaredSize, tag, webhookURL,
	)
	if err != nil {
		return fmt.Errorf("erro ao inserir vídeo: %w", err)
	}
	return nil
}

// DeleteVideo remove o registro do vídeo do banco. Os tokens de acesso e
// variantes têm FK para videos; o chamador deve removê-los antes (ou o banco
// rejeitará). Ver admin.HandleDeleteVideo para a remoção completa (linhas + disco).
func DeleteVideo(db *sql.DB, videoID string) error {
	_, err := db.Exec("DELETE FROM videos WHERE video_id = ?", videoID)
	if err != nil {
		return fmt.Errorf("erro ao apagar vídeo: %w", err)
	}
	return nil
}

// GetVideo busca um vídeo pelo seu ID e retorna a struct preenchida.
// Trata corretamente os campos que podem ser NULL no banco.
func GetVideo(db *sql.DB, videoID string) (*Video, error) {
	row := db.QueryRow(
		`SELECT `+SelectVideoColumns+` FROM videos WHERE video_id = ?`,
		videoID,
	)
	// Repassa sql.ErrNoRows sem embrulhar para o chamador poder identificar
	return ScanVideoRow(row.Scan)
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
		`SELECT `+SelectVideoColumns+` FROM videos WHERE status = ?`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar vídeos por status: %w", err)
	}
	defer rows.Close()

	var videos []*Video
	for rows.Next() {
		v, err := ScanVideoRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler linha de vídeo: %w", err)
		}
		videos = append(videos, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar vídeos: %w", err)
	}

	return videos, nil
}
