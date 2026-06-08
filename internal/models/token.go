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
	// ProjectID vincula o token a um projeto interno (issue #6, T33). É nil
	// para tokens do fluxo legado, gerados a partir da chave global
	// UPLOAD_TOKEN_SECRET (sem projeto associado).
	ProjectID *int64
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

// InsertUploadToken persiste um novo token de upload no banco, sem projeto
// associado (fluxo legado, chave global UPLOAD_TOKEN_SECRET).
// Retorna erro se o video_id não existir (foreign key) ou se já houver
// um token para o mesmo video_id (UNIQUE constraint).
func InsertUploadToken(db *sql.DB, token, videoID string, expiresAt time.Time) error {
	return InsertUploadTokenForProject(db, token, videoID, expiresAt, nil)
}

// InsertUploadTokenForProject persiste um token de upload vinculado a um
// projeto (issue #6, T33) — projectID nil equivale ao fluxo legado.
func InsertUploadTokenForProject(db *sql.DB, token, videoID string, expiresAt time.Time, projectID *int64) error {
	_, err := db.Exec(
		`INSERT INTO upload_tokens (token, video_id, expires_at, project_id) VALUES (?, ?, ?, ?)`,
		token, videoID, expiresAt.UTC().Format("2006-01-02 15:04:05"), projectID,
	)
	return err
}

// scanUploadToken lê uma linha de upload_tokens para a struct, tratando
// project_id (nullable — fluxo legado não tem projeto associado).
func scanUploadToken(scan func(dest ...any) error) (*UploadToken, error) {
	var t UploadToken
	var expiresAt string
	var projectID sql.NullInt64
	if err := scan(&t.Token, &t.VideoID, &expiresAt, &projectID); err != nil {
		return nil, err
	}
	t.ExpiresAt = parseDateTime(expiresAt)
	if projectID.Valid {
		t.ProjectID = &projectID.Int64
	}
	return &t, nil
}

// GetUploadToken busca um token pelo valor do token.
// Retorna sql.ErrNoRows se não encontrado.
func GetUploadToken(db *sql.DB, token string) (*UploadToken, error) {
	row := db.QueryRow(`SELECT token, video_id, expires_at, project_id FROM upload_tokens WHERE token = ?`, token)
	return scanUploadToken(row.Scan)
}

// GetUploadTokenByVideoID busca o token ativo para um video_id.
// Retorna sql.ErrNoRows se não encontrado.
func GetUploadTokenByVideoID(db *sql.DB, videoID string) (*UploadToken, error) {
	row := db.QueryRow(`SELECT token, video_id, expires_at, project_id FROM upload_tokens WHERE video_id = ?`, videoID)
	return scanUploadToken(row.Scan)
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
