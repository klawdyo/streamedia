// Pacote transcode implementa o worker que transcodifica vídeos para HLS
// usando FFmpeg, gera o master playlist e atualiza o estado no banco.
package transcode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// FFmpegExecutor abstrai a execução do FFmpeg para permitir mock nos testes.
type FFmpegExecutor interface {
	Run(ctx context.Context, args []string) error
}

// RealFFmpeg é a implementação real que invoca o binário ffmpeg.
type RealFFmpeg struct{}

// Run executa o ffmpeg com os argumentos fornecidos, respeitando o contexto
// (timeout/cancelamento).
func (r *RealFFmpeg) Run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	return cmd.Run()
}

// Worker encapsula a configuração, conexão ao banco, executor FFmpeg e o
// callback de webhook usado para notificar o resultado da transcodificação.
type Worker struct {
	cfg       *config.Config
	db        *sql.DB
	ffmpeg    FFmpegExecutor
	onWebhook func(videoID, event, errMsg string)
}

// NewWorker cria um Worker com o executor real do FFmpeg.
func NewWorker(cfg *config.Config, db *sql.DB, onWebhook func(videoID, event, errMsg string)) *Worker {
	return &Worker{
		cfg:       cfg,
		db:        db,
		ffmpeg:    &RealFFmpeg{},
		onWebhook: onWebhook,
	}
}

// resolutionProfile define os parâmetros de codificação por resolução.
type resolutionProfile struct {
	width        int    // largura do scale
	height       int    // altura do scale (o "p")
	videoBitrate string // bitrate de vídeo, ex.: "900k"
	audioBitrate string // bitrate de áudio, ex.: "128k"
	bandwidth    int    // BANDWIDTH para o master playlist
}

// profiles mapeia cada resolução suportada para seu perfil de codificação.
var profiles = map[int]resolutionProfile{
	480:  {width: 854, height: 480, videoBitrate: "900k", audioBitrate: "128k", bandwidth: 900000},
	720:  {width: 1280, height: 720, videoBitrate: "2000k", audioBitrate: "128k", bandwidth: 2000000},
	1080: {width: 1920, height: 1080, videoBitrate: "3500k", audioBitrate: "192k", bandwidth: 3500000},
}

// determineResolutions decide quais resoluções de saída gerar com base nas
// dimensões de origem. Usa o MÍNIMO entre largura e altura como o valor "p"
// (a menor dimensão é a que define a resolução real percebida, funcionando
// tanto para vídeos landscape quanto portrait). Limita o teto em 1080p.
func determineResolutions(originWidth, originHeight int) []int {
	// A menor dimensão representa o "p" do vídeo.
	minDim := originWidth
	if originHeight < minDim {
		minDim = originHeight
	}

	var resolutions []int
	if minDim >= 480 {
		resolutions = append(resolutions, 480)
	}
	if minDim >= 720 {
		resolutions = append(resolutions, 720)
	}
	if minDim >= 1080 {
		resolutions = append(resolutions, 1080)
	}
	return resolutions
}

// generateMasterM3U8 gera o conteúdo do master playlist HLS, referenciando
// o playlist de cada resolução com seu BANDWIDTH e RESOLUTION.
func generateMasterM3U8(resolutions []int) string {
	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:3\n")

	for _, res := range resolutions {
		p, ok := profiles[res]
		if !ok {
			// Resolução desconhecida: ignora silenciosamente.
			continue
		}
		// Linha de stream-inf com bandwidth e resolução.
		sb.WriteString(fmt.Sprintf(
			"#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n",
			p.bandwidth, p.width, p.height,
		))
		// Path relativo do playlist da variante.
		sb.WriteString(fmt.Sprintf("%d/playlist.m3u8\n", res))
	}

	return sb.String()
}

// buildFFmpegArgs monta os argumentos do FFmpeg para transcodificar o vídeo
// de entrada para uma variante HLS na resolução informada.
func buildFFmpegArgs(input, outputDir string, resolution int) []string {
	p, ok := profiles[resolution]
	if !ok {
		// Fallback para 480p caso a resolução seja desconhecida.
		p = profiles[480]
	}

	// Diretório de saída específico da resolução.
	resDir := filepath.Join(outputDir, strconv.Itoa(resolution))

	return []string{
		"-i", input,
		"-vf", fmt.Sprintf("scale=%d:%d", p.width, p.height),
		"-c:v", "libx264",
		"-b:v", p.videoBitrate,
		"-c:a", "aac",
		"-b:a", p.audioBitrate,
		"-hls_time", "6",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(resDir, "%d.ts"),
		"-f", "hls",
		filepath.Join(resDir, "playlist.m3u8"),
	}
}

// probeResult contém as dimensões e duração extraídas do vídeo de origem.
type probeResult struct {
	width     int
	height    int
	durationS int
}

// ffprobeOutput espelha a parte relevante do JSON retornado pelo ffprobe.
type ffprobeOutput struct {
	Streams []struct {
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Duration string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// probeVideo executa ffprobe para obter dimensões e duração do vídeo.
// Em caso de qualquer falha (ffprobe ausente, arquivo inválido) retorna um
// padrão seguro de 480p (854x480) para não bloquear o pipeline.
func probeVideo(path string) *probeResult {
	// Padrão seguro caso o ffprobe falhe.
	def := &probeResult{width: 854, height: 480}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return def
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return def
	}

	// Procura o primeiro stream com dimensões válidas (o stream de vídeo).
	for _, s := range parsed.Streams {
		if s.Width > 0 && s.Height > 0 {
			res := &probeResult{width: s.Width, height: s.Height}
			// Tenta a duração do stream, com fallback para o format.
			if d := parseDurationSeconds(s.Duration); d > 0 {
				res.durationS = d
			} else {
				res.durationS = parseDurationSeconds(parsed.Format.Duration)
			}
			return res
		}
	}

	return def
}

// parseDurationSeconds converte a duração em segundos (string com possíveis
// casas decimais) para um inteiro de segundos. Retorna 0 se inválida.
func parseDurationSeconds(s string) int {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(f)
}

// renditionSegmentRe casa nomes de segmento gerados pelo FFmpeg para HLS:
// um ou mais dígitos seguidos de ".ts" (mesmo padrão usado no serving,
// ver internal/serve.segmentRe).
var renditionSegmentRe = regexp.MustCompile(`^[0-9]+\.ts$`)

// scanRenditionDir varre o diretório de uma variante HLS recém-gerada e
// soma o tamanho dos segmentos .ts, contando-os — alimenta video_renditions
// (issue #5, T36). Ignora o playlist.m3u8 deliberadamente: o pedido da
// issue é "tamanho dos segmentos da variante", e o playlist é uma fração
// desprezível e variável (não representa o "peso" real do vídeo).
func scanRenditionDir(resDir string) (sizeBytes int64, segmentCount int, err error) {
	entries, err := os.ReadDir(resDir)
	if err != nil {
		return 0, 0, fmt.Errorf("erro ao ler diretório da variante %s: %w", resDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !renditionSegmentRe.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, 0, fmt.Errorf("erro ao obter informações do segmento %s: %w", entry.Name(), err)
		}
		sizeBytes += info.Size()
		segmentCount++
	}

	return sizeBytes, segmentCount, nil
}

// Transcode processa um vídeo: extrai dimensões, gera as variantes HLS,
// escreve o master playlist, marca como pronto e dispara o webhook.
// Em caso de falha trata as tentativas e o estado conforme as regras.
func (w *Worker) Transcode(videoID string) error {
	// 1. Busca o vídeo (com contador de tentativas).
	video, err := models.GetVideo(w.db, videoID)
	if err != nil {
		return fmt.Errorf("erro ao buscar vídeo %s: %w", videoID, err)
	}

	// 2. Garante o estado transcoding (transição válida a partir de upload_complete).
	if video.Status != models.StatusTranscoding {
		if err := models.UpdateStatus(w.db, videoID, models.StatusTranscoding); err != nil {
			return fmt.Errorf("erro ao transicionar para transcoding: %w", err)
		}
	}

	// 3. Caminho do arquivo de entrada original.
	inputPath := filepath.Join(w.cfg.UploadTmpDir, videoID)

	// 4. Extrai dimensões e duração (com padrão seguro se ffprobe falhar).
	probe := probeVideo(inputPath)

	// 5. Determina quais resoluções gerar.
	resolutions := determineResolutions(probe.width, probe.height)

	// 6. Diretório de saída base do vídeo — isolado por projeto (issue #6,
	// T34): <MEDIA_DIR>/<slug-do-projeto>/<video_id>/. ProjectID nil
	// resolve para "" (layout legado, sem prefixo de projeto).
	rootDir, err := models.ResolveVideoRootDir(w.db, video.ProjectID)
	if err != nil {
		return fmt.Errorf("erro ao resolver diretório do projeto para %s: %w", videoID, err)
	}
	outputDir := filepath.Join(w.cfg.MediaDir, rootDir, videoID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return w.handleTranscodeFailure(videoID, video.TranscodeAttempts,
			fmt.Sprintf("erro ao criar diretório de saída: %v", err))
	}

	// 7. Transcodifica cada resolução.
	for _, res := range resolutions {
		resDir := filepath.Join(outputDir, strconv.Itoa(res))
		if err := os.MkdirAll(resDir, 0755); err != nil {
			return w.handleTranscodeFailure(videoID, video.TranscodeAttempts,
				fmt.Sprintf("erro ao criar diretório da resolução %d: %v", res, err))
		}

		args := buildFFmpegArgs(inputPath, outputDir, res)

		// Timeout generoso por variante para vídeos longos.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := w.ffmpeg.Run(ctx, args)
		cancel()

		if err != nil {
			return w.handleTranscodeFailure(videoID, video.TranscodeAttempts,
				fmt.Sprintf("erro ao transcodificar resolução %d: %v", res, err))
		}

		// Registra o tamanho e a contagem de segmentos da variante recém
		// gerada (issue #5, T36) — alimenta as agregações de armazenamento
		// em internal/models/storage.go. Falha aqui não compromete o vídeo
		// (estatísticas são um recurso auxiliar): logamos e seguimos.
		sizeBytes, segmentCount, err := scanRenditionDir(resDir)
		if err != nil {
			log.Printf("[transcode] %s: erro ao calcular tamanho da variante %dp: %v", videoID, res, err)
		} else if err := models.UpsertVideoRendition(w.db, videoID, res, sizeBytes, segmentCount); err != nil {
			log.Printf("[transcode] %s: erro ao registrar variante %dp: %v", videoID, res, err)
		}
	}

	// 8. Escreve o master playlist.
	master := generateMasterM3U8(resolutions)
	masterPath := filepath.Join(outputDir, "master.m3u8")
	if err := os.WriteFile(masterPath, []byte(master), 0644); err != nil {
		return w.handleTranscodeFailure(videoID, video.TranscodeAttempts,
			fmt.Sprintf("erro ao escrever master.m3u8: %v", err))
	}

	// 9. Marca o vídeo como pronto.
	if err := models.SetReady(w.db, videoID, probe.durationS, resolutions); err != nil {
		return w.handleTranscodeFailure(videoID, video.TranscodeAttempts,
			fmt.Sprintf("erro ao marcar vídeo como pronto: %v", err))
	}

	// 10. Remove o original (e seu .info) se não for para manter.
	if !w.cfg.KeepOriginal {
		_ = os.Remove(inputPath)
		_ = os.Remove(inputPath + ".info")
	}

	// 11. Notifica sucesso via webhook.
	w.onWebhook(videoID, "ready", "")

	// 12. Sucesso.
	return nil
}

// handleTranscodeFailure incrementa as tentativas e decide se a falha é
// terminal. Quando as tentativas atingem o máximo, marca como
// failed_transcode e dispara o webhook "failed" (retornando nil para não
// reenfileirar). Caso contrário, retorna erro para permitir nova tentativa.
func (w *Worker) handleTranscodeFailure(videoID string, currentAttempts int, errMsg string) error {
	// Incrementa o contador de tentativas (best-effort).
	_ = models.IncrementTranscodeAttempts(w.db, videoID)

	// Se atingiu (ou superou) o máximo, é falha terminal.
	if currentAttempts+1 >= w.cfg.MaxTranscodeAttempts {
		_ = models.UpdateStatusWithError(w.db, videoID, models.StatusFailedTranscode, errMsg)
		w.onWebhook(videoID, "failed", errMsg)
		return nil
	}

	// Ainda há tentativas: retorna erro para reprocessamento.
	return fmt.Errorf("%s", errMsg)
}
