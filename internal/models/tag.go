package models

import (
	"strings"
	"unicode"
)

// Slugify normaliza uma tag (namespace de um vídeo) em um slug: minúsculas,
// sem acentos, espaços e separadores convertidos em "-", e apenas caracteres
// em [a-z0-9-] preservados. Múltiplos "-" consecutivos são colapsados e "-"
// nas extremidades são removidos.
//
// O resultado é usado tanto como rótulo de namespace quanto como nome do
// diretório no disco (<MEDIA_DIR>/<tag>/<video_id>/...) — por isso precisa
// ser estável e seguro para uso em paths (neutraliza tentativas de traversal
// como "../"): qualquer caractere fora de [a-z0-9] vira separador e some.
func Slugify(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(stripDiacritics(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// stripDiacritics remove acentos (á → a, ç → c, ã → a, ...) por
// decomposição Unicode + remoção de marcas de combinação — evita depender
// de uma tabela de transliteração mantida manualmente.
func stripDiacritics(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) { // marca de combinação (acento)
			continue
		}
		b.WriteRune(r)
	}
	return decompose(b.String())
}

// accentReplacer é criado uma única vez e reutilizado em todas as chamadas
// de decompose. strings.NewReplacer é thread-safe. Cobre o alfabeto latino
// estendido usado em nomes em português; não pretende ser NFD completo.
var accentReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "õ", "o", "ô", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
	"Á", "A", "À", "A", "Ã", "A", "Â", "A", "Ä", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Õ", "O", "Ô", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

func decompose(s string) string {
	return accentReplacer.Replace(s)
}
