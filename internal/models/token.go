package models

import (
	"database/sql"
	"time"
)

// Propósitos possíveis de um token de acesso efêmero. O propósito é validado
// no uso: um token de play nunca autoriza upload e vice-versa.
const (
	PurposeUpload = "upload"
	PurposePlay   = "play"
)

// AccessToken representa um token efêmero de acesso a um vídeo — de upload
// (autoriza o envio dos bytes via TUS) ou de play (autoriza a leitura do
// master.m3u8). É uma string aleatória opaca validada por lookup; o `Purpose`
// separa os dois usos. UNIQUE(video_id, purpose) garante no máximo um token
// ativo de cada propósito por vídeo.
type AccessToken struct {
	Token     string
	VideoID   string
	Purpose   string
	ExpiresAt time.Time
}

// IsExpired verifica se o token já passou do prazo de validade.
func (t *AccessToken) IsExpired() bool {
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

// InsertAccessToken persiste um token de acesso. Usa INSERT OR REPLACE para
// que reemitir um token do mesmo (video_id, purpose) substitua o anterior —
// é a rotação natural: o token antigo morre na hora. Retorna erro se o
// video_id não existir (foreign key).
func InsertAccessToken(db *sql.DB, token, videoID, purpose string, expiresAt time.Time) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO access_tokens (token, video_id, purpose, expires_at) VALUES (?, ?, ?, ?)`,
		token, videoID, purpose, expiresAt.UTC().Format("2006-01-02 15:04:05"),
	)
	return err
}

// scanAccessToken lê uma linha de access_tokens para a struct.
func scanAccessToken(scan func(dest ...any) error) (*AccessToken, error) {
	var t AccessToken
	var expiresAt string
	if err := scan(&t.Token, &t.VideoID, &t.Purpose, &expiresAt); err != nil {
		return nil, err
	}
	t.ExpiresAt = parseDateTime(expiresAt)
	return &t, nil
}

// GetAccessToken busca um token pelo seu valor. Retorna sql.ErrNoRows se não
// encontrado. O chamador deve verificar Purpose e IsExpired.
func GetAccessToken(db *sql.DB, token string) (*AccessToken, error) {
	row := db.QueryRow(`SELECT token, video_id, purpose, expires_at FROM access_tokens WHERE token = ?`, token)
	return scanAccessToken(row.Scan)
}

// DeleteExpiredTokens remove todos os tokens (de qualquer propósito) com
// expires_at no passado. Retorna o número de tokens deletados.
func DeleteExpiredTokens(db *sql.DB) (int64, error) {
	result, err := db.Exec(`DELETE FROM access_tokens WHERE datetime(expires_at) < datetime('now')`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteAccessTokensForVideo remove todos os tokens de um vídeo — usado ao
// apagar o vídeo (limpa as credenciais efêmeras associadas).
func DeleteAccessTokensForVideo(db *sql.DB, videoID string) error {
	_, err := db.Exec(`DELETE FROM access_tokens WHERE video_id = ?`, videoID)
	return err
}
