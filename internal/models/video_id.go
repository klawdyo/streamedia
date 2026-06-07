package models

import (
	"regexp"

	"github.com/google/uuid"
)

// uuidFormatRe casa qualquer UUID bem-formado (RFC 4122): 8-4-4-4-12 hex,
// nibble de versão entre 1 e 8 (versões definidas pela RFC) e nibble de
// variante entre 8, 9, a ou b (variante RFC 4122). Continua rejeitando
// qualquer string que não seja um UUID — proteção essencial contra path
// traversal, já que video_id vira nome de diretório/arquivo em todo o
// sistema (serving HLS, transcodificação, uploads).
var uuidFormatRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// IsValidVideoIDFormat valida que s é um UUID bem-formado de qualquer versão
// suportada pela RFC 4122 (v1 a v8). Não exige uma versão específica — o
// sistema aceita qualquer versão informada pelo cliente e gera v7 quando o
// próprio sistema cria o id.
func IsValidVideoIDFormat(s string) bool {
	return uuidFormatRe.MatchString(s)
}

// NewVideoID gera um novo identificador de vídeo. O sistema sempre privilegia
// UUID v7 ao gerar ids — é ordenável por tempo de criação (prefixo temporal),
// o que melhora localidade no índice B-tree do SQLite e facilita ordenação
// cronológica natural por id.
func NewVideoID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
