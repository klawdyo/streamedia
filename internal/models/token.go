package models

import (
	"database/sql"
	"time"
)

// UploadToken representa um token de autorização para upload TUS.
// Cada token está vinculado a exatamente um video_id (constraint UNIQUE no banco).
type UploadToken struct {
	Token     string
	VideoID   string
	ExpiresAt time.Time
}

// IsExpired verifica se o token já passou do prazo de validade.
func (t *UploadToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// parseDateTime interpreta a string de datetime armazenada pelo SQLite,
// tentando os formatos mais comuns. Retorna o tempo em UTC.
func parseDateTime(s string) time.Time {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05+00:00",
		"2006-01-02 15:04:05.999999999-07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// InsertUploadToken persiste um novo token de upload no banco.
// Retorna erro se o video_id não existir (foreign key) ou se já houver
// um token para o mesmo video_id (UNIQUE constraint).
func InsertUploadToken(db *sql.DB, token, videoID string, expiresAt time.Time) error {
	_, err := db.Exec(
		`INSERT INTO upload_tokens (token, video_id, expires_at) VALUES (?, ?, ?)`,
		token, videoID, expiresAt.UTC().Format("2006-01-02 15:04:05"),
	)
	return err
}

// GetUploadToken busca um token pelo valor do token.
// Retorna sql.ErrNoRows se não encontrado.
func GetUploadToken(db *sql.DB, token string) (*UploadToken, error) {
	var t UploadToken
	var expiresAt string
	err := db.QueryRow(
		`SELECT token, video_id, expires_at FROM upload_tokens WHERE token = ?`,
		token,
	).Scan(&t.Token, &t.VideoID, &expiresAt)
	if err != nil {
		return nil, err
	}
	t.ExpiresAt = parseDateTime(expiresAt)
	return &t, nil
}

// GetUploadTokenByVideoID busca o token ativo para um video_id.
// Retorna sql.ErrNoRows se não encontrado.
func GetUploadTokenByVideoID(db *sql.DB, videoID string) (*UploadToken, error) {
	var t UploadToken
	var expiresAt string
	err := db.QueryRow(
		`SELECT token, video_id, expires_at FROM upload_tokens WHERE video_id = ?`,
		videoID,
	).Scan(&t.Token, &t.VideoID, &expiresAt)
	if err != nil {
		return nil, err
	}
	t.ExpiresAt = parseDateTime(expiresAt)
	return &t, nil
}

// DeleteUploadToken remove um token pelo seu valor.
func DeleteUploadToken(db *sql.DB, token string) error {
	_, err := db.Exec(`DELETE FROM upload_tokens WHERE token = ?`, token)
	return err
}

// DeleteExpiredTokens remove todos os tokens com expires_at no passado.
// Retorna o número de tokens deletados.
func DeleteExpiredTokens(db *sql.DB) (int64, error) {
	result, err := db.Exec(`DELETE FROM upload_tokens WHERE datetime(expires_at) < datetime('now')`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
