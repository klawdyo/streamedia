package transcode

import (
	"context"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"github.com/klawdyo/streamedia/internal/models"
)

// jpegQScale é o valor de qualidade passado ao encoder mjpeg do FFmpeg ao
// gerar os thumbnails. O mjpeg usa a escala de qualidade do FFmpeg, de 1
// (melhor) a 31 (pior) — e NÃO a escala 0–100 do libjpeg. Cada passo equivale
// a aproximadamente ~5% de qualidade percebida; q=5 corresponde a cerca de
// 80% de qualidade JPEG, o alvo pedido pela issue #19. É uma aproximação
// deliberada: o FFmpeg não expõe a escala 0–100 para o mjpeg.
const jpegQScale = "5"

// thumbnailScale calcula as dimensões (largura×altura) do thumbnail para uma
// resolução alvo, preservando a proporção original do vídeo. A MENOR dimensão
// da saída recebe o valor da resolução (o "p"), espelhando determineResolutions:
// assim um vídeo landscape 16:9 em 480p vira 854×480 e um portrait 9:16 vira
// 480×854. Garante dimensões pares (exigência comum de escaladores/encoders).
func thumbnailScale(originW, originH, resolution int) (w, h int) {
	// Sem dimensões válidas (ffprobe falhou): assume 16:9 landscape como padrão
	// seguro — o mesmo espírito do fallback de probeVideo.
	if originW <= 0 || originH <= 0 {
		originW, originH = 16, 9
	}

	if originW >= originH {
		// Landscape (ou quadrado): a altura é o "p"; largura proporcional.
		h = resolution
		w = int(math.Round(float64(resolution) * float64(originW) / float64(originH)))
	} else {
		// Portrait: a largura é o "p"; altura proporcional.
		w = resolution
		h = int(math.Round(float64(resolution) * float64(originH) / float64(originW)))
	}

	// Arredonda cada dimensão para o próximo número par.
	if w%2 != 0 {
		w++
	}
	if h%2 != 0 {
		h++
	}
	return w, h
}

// buildThumbnailArgs monta os argumentos do FFmpeg para extrair um único frame
// do vídeo de entrada e gravá-lo como JPEG no caminho de saída, escalado para
// (w×h). seekSeconds controla o ponto de extração: 1 (frame representativo a
// 1s) ou 0 (fallback para o primeiro frame). O seek vem ANTES do -i (input
// seeking): é rápido e cai no keyframe mais próximo do ponto pedido — que é
// exatamente o "fallback para o primeiro keyframe" descrito na issue #19.
func buildThumbnailArgs(input, outputPath string, w, h, seekSeconds int) []string {
	return []string{
		"-y",                             // sobrescreve a saída sem perguntar
		"-ss", strconv.Itoa(seekSeconds), // busca o ponto de extração (input seeking)
		"-i", input,
		"-frames:v", "1", // extrai exatamente 1 frame
		"-vf", fmt.Sprintf("scale=%d:%d", w, h), // reescala preservando o tamanho calculado
		"-q:v", jpegQScale, // qualidade JPEG (~80%)
		"-f", "image2", // muxer de imagem única
		outputPath,
	}
}

// generateThumbnails gera um thumbnail JPEG por resolução a partir do vídeo de
// entrada ORIGINAL, gravando em <outputDir>/thumb_<res>.jpg. É best-effort:
// thumbnails são um recurso auxiliar (poster), então qualquer falha aqui é
// apenas logada e nunca compromete a transcodificação já concluída — mesmo
// princípio do scanRenditionDir (estatísticas, T36).
func (w *Worker) generateThumbnails(videoID, inputPath, outputDir string, originW, originH int, resolutions []int) {
	for _, res := range resolutions {
		tw, th := thumbnailScale(originW, originH, res)
		outputPath := filepath.Join(outputDir, models.ThumbnailFileName(res))

		// Tenta extrair o frame a 1s; se falhar (vídeo < 1s, sem frame nesse
		// ponto), refaz buscando o primeiro frame (0s).
		if err := w.runThumbnail(inputPath, outputPath, tw, th, 1); err != nil {
			if err := w.runThumbnail(inputPath, outputPath, tw, th, 0); err != nil {
				log.Printf("[thumbnail] %s: falha ao gerar thumbnail %dp: %v", videoID, res, err)
			}
		}
	}
}

// runThumbnail executa o FFmpeg para um único thumbnail, com timeout curto
// (extrair um frame é uma operação barata).
func (w *Worker) runThumbnail(inputPath, outputPath string, tw, th, seekSeconds int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := buildThumbnailArgs(inputPath, outputPath, tw, th, seekSeconds)
	return w.ffmpeg.Run(ctx, args)
}
