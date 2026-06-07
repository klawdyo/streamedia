// Definições compartilhadas do formato HLS, reaproveitadas tanto pela
// geração (worker de transcodificação) quanto pelo serving estático.
// Centralizar aqui evita que as duas pontas do mesmo contrato divirjam
// com o tempo — se o FFmpeg mudar o padrão de nomenclatura, o ajuste
// acontece em um único lugar.
package models

import "regexp"

// SegmentNameRe casa nomes de segmento HLS gerados pelo FFmpeg: um ou mais
// dígitos seguidos de ".ts". Definição única — usada pelo worker para
// escanear o diretório de saída e pelo serving estático para validar
// nomes de arquivo antes de servir, garantindo que nenhum path inválido
// escape.
var SegmentNameRe = regexp.MustCompile(`^[0-9]+\.ts$`)
