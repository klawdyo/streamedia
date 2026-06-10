// Definições compartilhadas do formato HLS, reaproveitadas tanto pela
// geração (worker de transcodificação) quanto pelo serving estático.
// Centralizar aqui evita que as duas pontas do mesmo contrato divirjam
// com o tempo — se o FFmpeg mudar o padrão de nomenclatura, o ajuste
// acontece em um único lugar.
package models

import (
	"fmt"
	"regexp"
)

// SegmentNameRe casa nomes de segmento HLS gerados pelo FFmpeg: um ou mais
// dígitos seguidos de ".ts". Definição única — usada pelo worker para
// escanear o diretório de saída e pelo serving estático para validar
// nomes de arquivo antes de servir, garantindo que nenhum path inválido
// escape.
var SegmentNameRe = regexp.MustCompile(`^[0-9]+\.ts$`)

// ThumbnailNameRe casa o nome de arquivo de um thumbnail (poster) por
// resolução: "thumb_<res>.jpg", restrito às resoluções suportadas
// (480/720/1080). Definição única — usada pela geração no worker (issue #19)
// e pelo serving público para validar o nome antes de servir o arquivo,
// bloqueando qualquer path inválido.
var ThumbnailNameRe = regexp.MustCompile(`^thumb_(480|720|1080)\.jpg$`)

// ThumbnailFileName devolve o nome de arquivo do thumbnail de uma resolução,
// no formato esperado por ThumbnailNameRe: "thumb_<res>.jpg".
func ThumbnailFileName(resolution int) string {
	return fmt.Sprintf("thumb_%d.jpg", resolution)
}
