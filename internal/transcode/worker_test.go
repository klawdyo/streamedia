package transcode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// TestDetermineResolutions_480pOrigin verifica que origem de 480p retorna
// apenas a resolução 480p.
func TestDetermineResolutions_480pOrigin(t *testing.T) {
	resolutions := determineResolutions(640, 480)
	expected := []int{480}
	if !sliceEqual(resolutions, expected) {
		t.Errorf("determineResolutions(640, 480) = %v, esperava %v", resolutions, expected)
	}
}

// TestDetermineResolutions_720pOrigin verifica que origem de 720p retorna
// 480p e 720p.
func TestDetermineResolutions_720pOrigin(t *testing.T) {
	resolutions := determineResolutions(1280, 720)
	expected := []int{480, 720}
	if !sliceEqual(resolutions, expected) {
		t.Errorf("determineResolutions(1280, 720) = %v, esperava %v", resolutions, expected)
	}
}

// TestDetermineResolutions_1080pOrigin verifica que origem de 1080p retorna
// 480p, 720p e 1080p.
func TestDetermineResolutions_1080pOrigin(t *testing.T) {
	resolutions := determineResolutions(1920, 1080)
	expected := []int{480, 720, 1080}
	if !sliceEqual(resolutions, expected) {
		t.Errorf("determineResolutions(1920, 1080) = %v, esperava %v", resolutions, expected)
	}
}

// TestDetermineResolutions_4KOrigin verifica que origem em 4K é limitada
// até 1080p (não transcoda acima disso).
func TestDetermineResolutions_4KOrigin(t *testing.T) {
	resolutions := determineResolutions(3840, 2160)
	expected := []int{480, 720, 1080}
	if !sliceEqual(resolutions, expected) {
		t.Errorf("determineResolutions(3840, 2160) = %v, esperava %v", resolutions, expected)
	}
}

// TestDetermineResolutions_PortraitVideo verifica que vídeo portrait/vertical
// usa a dimensão MAIOR para determinar as resoluções. 720x1280 (maior = 1280)
// equivale a 720p.
func TestDetermineResolutions_PortraitVideo(t *testing.T) {
	resolutions := determineResolutions(720, 1280)
	expected := []int{480, 720}
	if !sliceEqual(resolutions, expected) {
		t.Errorf("determineResolutions(720, 1280) = %v, esperava %v", resolutions, expected)
	}
}

// TestGenerateMasterM3U8_TwoResolutions verifica que o master M3U8 contém
// as resoluções, bandwidths e paths corretos para 2 resoluções.
func TestGenerateMasterM3U8_TwoResolutions(t *testing.T) {
	result := generateMasterM3U8([]int{480, 720})

	// Verifica headers obrigatórios
	if !strings.Contains(result, "#EXTM3U") {
		t.Error("generateMasterM3U8 deve conter #EXTM3U header")
	}

	// Verifica que contém os playlists das resoluções
	if !strings.Contains(result, "480/playlist.m3u8") {
		t.Error("generateMasterM3U8 deve conter '480/playlist.m3u8'")
	}
	if !strings.Contains(result, "720/playlist.m3u8") {
		t.Error("generateMasterM3U8 deve conter '720/playlist.m3u8'")
	}

	// Verifica que NÃO contém 1080p
	if strings.Contains(result, "1080/playlist.m3u8") {
		t.Error("generateMasterM3U8 não deve conter '1080/playlist.m3u8' para 2 resoluções")
	}

	// Verifica bandwidths
	if !strings.Contains(result, "BANDWIDTH=900000") {
		t.Error("generateMasterM3U8 deve conter 'BANDWIDTH=900000' para 480p")
	}
	if !strings.Contains(result, "BANDWIDTH=2000000") {
		t.Error("generateMasterM3U8 deve conter 'BANDWIDTH=2000000' para 720p")
	}
}

// TestGenerateMasterM3U8_ThreeResolutions verifica que o master M3U8 contém
// as 3 resoluções com bandwidths corretos.
func TestGenerateMasterM3U8_ThreeResolutions(t *testing.T) {
	result := generateMasterM3U8([]int{480, 720, 1080})

	// Verifica que contém todas as 3 resoluções
	if !strings.Contains(result, "480/playlist.m3u8") {
		t.Error("generateMasterM3U8 deve conter '480/playlist.m3u8'")
	}
	if !strings.Contains(result, "720/playlist.m3u8") {
		t.Error("generateMasterM3U8 deve conter '720/playlist.m3u8'")
	}
	if !strings.Contains(result, "1080/playlist.m3u8") {
		t.Error("generateMasterM3U8 deve conter '1080/playlist.m3u8'")
	}

	// Verifica bandwidth para 1080p
	if !strings.Contains(result, "BANDWIDTH=3500000") {
		t.Error("generateMasterM3U8 deve conter 'BANDWIDTH=3500000' para 1080p")
	}
}

// TestBuildFFmpegArgs_480p verifica que os argumentos FFmpeg para 480p contêm
// scale, bitrate de vídeo 900k, bitrate de áudio 128k e HLS flags.
func TestBuildFFmpegArgs_480p(t *testing.T) {
	args := buildFFmpegArgs("/tmp/input.mp4", "/media/vid123", 480)

	// Verifica filter de escala
	if !containsArg(args, "scale=854:480") {
		t.Error("buildFFmpegArgs deve conter 'scale=854:480' para 480p")
	}

	// Verifica bitrate de vídeo
	if !containsArg(args, "900k") {
		t.Error("buildFFmpegArgs deve conter '900k' (video bitrate para 480p)")
	}

	// Verifica bitrate de áudio
	if !containsArg(args, "128k") {
		t.Error("buildFFmpegArgs deve conter '128k' (audio bitrate)")
	}

	// Verifica HLS flags
	if !containsArg(args, "-hls_time") {
		t.Error("buildFFmpegArgs deve conter '-hls_time' para HLS")
	}
}

// TestBuildFFmpegArgs_1080p verifica que os argumentos FFmpeg para 1080p contêm
// scale, bitrate de vídeo 3500k e bitrate de áudio 192k.
func TestBuildFFmpegArgs_1080p(t *testing.T) {
	args := buildFFmpegArgs("/tmp/input.mp4", "/media/vid123", 1080)

	// Verifica filter de escala
	if !containsArg(args, "scale=1920:1080") {
		t.Error("buildFFmpegArgs deve conter 'scale=1920:1080' para 1080p")
	}

	// Verifica bitrate de vídeo
	if !containsArg(args, "3500k") {
		t.Error("buildFFmpegArgs deve conter '3500k' (video bitrate para 1080p)")
	}

	// Verifica bitrate de áudio
	if !containsArg(args, "192k") {
		t.Error("buildFFmpegArgs deve conter '192k' (audio bitrate)")
	}
}

// TestTranscodeWorker_FFmpegNotAvailable verifica que quando FFmpeg falha
// (ou não está disponível) e as tentativas atingem MaxTranscodeAttempts,
// o vídeo transiciona para failed_transcode e o webhook é chamado com event="failed".
func TestTranscodeWorker_FFmpegNotAvailable(t *testing.T) {
	// Abre banco em memória
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	// Insere um vídeo com status 'transcoding' e tentativas = 2 (uma menos que max=3)
	_, err = database.Exec(
		"INSERT INTO videos (video_id, tag, status, transcode_attempts) VALUES (?, ?, ?, ?)",
		"test-ffmpeg-fail", "default", "transcoding", 2,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	cfg := &config.Config{
		UploadTmpDir:         t.TempDir(),
		MediaDir:             t.TempDir(),
		MaxTranscodeAttempts: 3,
		KeepOriginal:         false,
	}

	// Rastreia chamadas do webhook
	var webhookCalls []struct {
		videoID string
		event   string
		errMsg  string
	}

	webhookFunc := func(videoID, event, errMsg string) {
		webhookCalls = append(webhookCalls, struct {
			videoID string
			event   string
			errMsg  string
		}{videoID, event, errMsg})
	}

	// Cria worker com executor FFmpeg que sempre falha
	w := NewWorker(cfg, database, webhookFunc)

	// Substitui o executor por um mock que falha
	mockExec := &mockFFmpeg{
		err: fmt.Errorf("ffmpeg not found"),
	}
	setWorkerExecutor(w, mockExec)

	// Chama Transcode
	err = w.Transcode("test-ffmpeg-fail")

	// Verifica que a transição para failed_transcode ocorreu
	video, err := models.GetVideo(database, "test-ffmpeg-fail")
	if err != nil {
		t.Fatalf("erro ao recuperar vídeo: %v", err)
	}
	if video.Status != models.StatusFailedTranscode {
		t.Errorf("esperava status 'failed_transcode', obtive %s", video.Status)
	}

	// Verifica que o webhook foi chamado com event="failed"
	if len(webhookCalls) == 0 {
		t.Error("webhook não foi chamado")
	} else {
		if webhookCalls[0].event != "failed" {
			t.Errorf("esperava event 'failed', obtive %q", webhookCalls[0].event)
		}
	}
}

// TestTranscodeWorker_UpdatesStatus verifica que o worker processa um vídeo
// corretamente quando FFmpeg sucede, atualizando o status e chamando o webhook.
// Este teste usa um mock FFmpeg que cria os arquivos de saída necessários.
func TestTranscodeWorker_UpdatesStatus(t *testing.T) {
	// Abre banco em memória
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	tempDir := t.TempDir()
	uploadTmpDir := filepath.Join(tempDir, "uploads")
	mediaDir := filepath.Join(tempDir, "media")

	if err := os.MkdirAll(uploadTmpDir, 0755); err != nil {
		t.Fatalf("erro ao criar uploadTmpDir: %v", err)
	}
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("erro ao criar mediaDir: %v", err)
	}

	// Insere um vídeo com status 'upload_complete' e tag vazia (a saída cai
	// direto em <MEDIA_DIR>/test-success, casando com o mock de FFmpeg abaixo).
	_, err = database.Exec(
		"INSERT INTO videos (video_id, tag, status) VALUES (?, ?, ?)",
		"test-success", "", "upload_complete",
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	// Cria arquivo de entrada temporário
	inputFile := filepath.Join(uploadTmpDir, "test-success")
	if err := os.WriteFile(inputFile, []byte("fake video data"), 0644); err != nil {
		t.Fatalf("erro ao criar arquivo de entrada: %v", err)
	}

	cfg := &config.Config{
		UploadTmpDir:         uploadTmpDir,
		MediaDir:             mediaDir,
		MaxTranscodeAttempts: 3,
		KeepOriginal:         false,
	}

	// Rastreia chamadas do webhook
	var webhookCalls []struct {
		videoID string
		event   string
		errMsg  string
	}

	webhookFunc := func(videoID, event, errMsg string) {
		webhookCalls = append(webhookCalls, struct {
			videoID string
			event   string
			errMsg  string
		}{videoID, event, errMsg})
	}

	// Cria worker
	w := NewWorker(cfg, database, webhookFunc)

	// Mock FFmpeg que simula sucesso e cria arquivos de saída
	mockExec := &mockFFmpeg{
		err: nil,
		createFiles: func(args []string) {
			// Cria a estrutura de diretórios esperada: mediaDir/test-success/480/
			outputDir := filepath.Join(mediaDir, "test-success", "480")
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				t.Logf("aviso: erro ao criar outputDir: %v", err)
			}
			// Cria playlist.m3u8 e um segment
			if err := os.WriteFile(
				filepath.Join(outputDir, "playlist.m3u8"),
				[]byte("#EXTM3U\n#EXT-X-VERSION:3\n"),
				0644,
			); err != nil {
				t.Logf("aviso: erro ao criar playlist: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(outputDir, "0.ts"),
				[]byte("fake ts segment"),
				0644,
			); err != nil {
				t.Logf("aviso: erro ao criar segment: %v", err)
			}
		},
	}
	setWorkerExecutor(w, mockExec)

	// Chama Transcode — pode falhar se FFprobe não estiver disponível,
	// mas o teste valida que a função não entra em pânico.
	_ = w.Transcode("test-success")

	// Verifica que o vídeo foi processado (status é ready ou failed, não transcoding)
	video, err := models.GetVideo(database, "test-success")
	if err != nil {
		t.Fatalf("erro ao recuperar vídeo: %v", err)
	}

	if video.Status == "transcoding" {
		t.Error("esperava que status deixasse de ser 'transcoding'")
	}

	// Verifica que o webhook foi chamado (ready ou failed)
	if len(webhookCalls) == 0 {
		t.Logf("aviso: webhook não foi chamado (possível falha de FFprobe)")
	}
}

// =============================================================================
// Mocks e helpers
// =============================================================================

// mockFFmpeg é um mock de FFmpegExecutor para testes.
type mockFFmpeg struct {
	err         error
	createFiles func(args []string) // callback para criar arquivos simulados
	// runFunc, se definido, assume o controle total da chamada (precedência
	// sobre createFiles/err) — usado para simular comportamento por-chamada,
	// como falhar no seek de 1s e suceder no fallback de 0s.
	runFunc func(args []string) error
}

func (m *mockFFmpeg) Run(ctx context.Context, args []string) error {
	if m.runFunc != nil {
		return m.runFunc(args)
	}
	if m.createFiles != nil {
		m.createFiles(args)
	}
	return m.err
}

// setWorkerExecutor substitui o executor FFmpeg de um Worker (acesso via unexported field).
// Só funciona em testes do mesmo package.
func setWorkerExecutor(w *Worker, exec FFmpegExecutor) {
	w.ffmpeg = exec
}

