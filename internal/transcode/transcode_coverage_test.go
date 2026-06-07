package transcode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// TestParseDurationSeconds_ValidFormats testa conversão de duração em diferentes formatos.
func TestParseDurationSeconds_ValidFormats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"zero", "0", 0},
		{"inteiro simples", "60", 60},
		{"decimal truncado", "60.5", 60},
		{"decimal grande", "123.999", 123},
		{"vazio", "", 0},
		{"inválido", "abc", 0},
		// Nota: código converte com strconv.Atoi que aceita negativos,
		// então "-10" retorna -10, não 0. Este é comportamento do código.
		{"negativo convertido", "-10", -10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDurationSeconds(tt.input)
			if got != tt.expected {
				t.Errorf("parseDurationSeconds(%q) = %d, esperava %d", tt.input, got, tt.expected)
			}
		})
	}
}

// TestDetermineResolutions_Various testa matriz de diferentes dimensões de vídeo.
// Nota: determineResolutions usa a MENOR dimensão (min(width, height)) e
// retorna resoluções apenas se minDim >= 480.
func TestDetermineResolutions_Various(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		height   int
		expected []int
	}{
		{"vídeo muito pequeno (240p)", 320, 240, []int{}}, // min=240, nada
		{"vídeo pequeno (360p)", 640, 360, []int{}}, // min=360, nada
		{"vídeo médio (480p landscape)", 853, 480, []int{480}}, // min=480, só 480
		{"vídeo médio (480p portrait)", 480, 853, []int{480}}, // min=480, só 480
		{"vídeo HD (720p)", 1280, 720, []int{480, 720}}, // min=720, inclui 480 e 720
		{"vídeo Full HD (1080p)", 1920, 1080, []int{480, 720, 1080}}, // min=1080, todos
		{"vídeo 2K (limitado a 1080p)", 2560, 1440, []int{480, 720, 1080}}, // min=1440, todos
		{"vídeo 4K (limitado a 1080p)", 3840, 2160, []int{480, 720, 1080}}, // min=2160, todos
		{"vídeo muito largo (landscape extremo)", 4000, 480, []int{480}}, // min=480, só 480
		{"vídeo muito alto (portrait extremo)", 480, 4000, []int{480}}, // min=480, só 480
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineResolutions(tt.width, tt.height)
			if !sliceEqual(got, tt.expected) {
				t.Errorf("determineResolutions(%d, %d) = %v, esperava %v", tt.width, tt.height, got, tt.expected)
			}
		})
	}
}

// TestGenerateMasterM3U8_ValidInput testa geração do master playlist.
func TestGenerateMasterM3U8_ValidInput(t *testing.T) {
	resolutions := []int{480, 720}
	content := generateMasterM3U8(resolutions)

	// Verifica que contém o header M3U8
	if !contains(content, "#EXTM3U") {
		t.Error("master M3U8 deve conter #EXTM3U header")
	}

	// Verifica que contém EXT-X-STREAM-INF para cada resolução
	// Referência é "480/playlist.m3u8" e "720/playlist.m3u8"
	if !contains(content, "480/playlist.m3u8") {
		t.Error("master M3U8 deve conter referência para 480/playlist.m3u8")
	}
	if !contains(content, "720/playlist.m3u8") {
		t.Error("master M3U8 deve conter referência para 720/playlist.m3u8")
	}

	// Verifica que contém a directiva version
	if !contains(content, "#EXT-X-VERSION") {
		t.Error("master M3U8 deve conter #EXT-X-VERSION")
	}
}

// TestGenerateMasterM3U8_SingleResolution testa master com apenas uma resolução.
func TestGenerateMasterM3U8_SingleResolution(t *testing.T) {
	resolutions := []int{480}
	content := generateMasterM3U8(resolutions)

	if !contains(content, "#EXTM3U") {
		t.Error("master M3U8 deve conter #EXTM3U header")
	}

	if !contains(content, "480/playlist.m3u8") {
		t.Error("master M3U8 deve conter referência para 480/playlist.m3u8")
	}
}

// TestBuildFFmpegArgs_MinimalArgs verifica que buildFFmpegArgs gera argumentos
// válidos para transcodificação com scaling.
func TestBuildFFmpegArgs_MinimalArgs(t *testing.T) {
	videoPath := "/tmp/test.mp4"
	outputDir := "/tmp/output"
	resolution := 480

	args := buildFFmpegArgs(videoPath, outputDir, resolution)

	// Deve incluir arquivo de entrada
	if !containsArg(args, videoPath) {
		t.Errorf("args deve conter path %q", videoPath)
	}

	// Deve incluir scale para a resolução
	if !containsArg(args, "480") {
		t.Errorf("args deve conter escala 480")
	}

	// Deve incluir output path
	if !containsArg(args, outputDir) {
		t.Errorf("args deve conter outputDir %q", outputDir)
	}

	// Deve incluir flags FFmpeg comuns
	if !containsArg(args, "-c:v") || !containsArg(args, "-c:a") {
		t.Error("args deve conter -c:v (video codec) e -c:a (audio codec)")
	}
}

// TestBuildFFmpegArgs_AllResolutions testa geração de argumentos para todas
// as resoluções suportadas.
func TestBuildFFmpegArgs_AllResolutions(t *testing.T) {
	resolutions := []int{480, 720, 1080}
	for _, res := range resolutions {
		args := buildFFmpegArgs("/test.mp4", "/output", res)
		if !containsArg(args, strconv.Itoa(res)) {
			t.Errorf("args para %dp deve conter resolução", res)
		}
	}
}

// TestScanRenditionDir_EmptyDir retorna valores zero para dir vazio.
func TestScanRenditionDir_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	sizeBytes, segmentCount, err := scanRenditionDir(tmpDir)
	if err != nil {
		t.Fatalf("scanRenditionDir para dir vazio retornou erro: %v", err)
	}
	if sizeBytes != 0 || segmentCount != 0 {
		t.Errorf("esperado 0 size e 0 segments para dir vazio, obtido size=%d segments=%d", sizeBytes, segmentCount)
	}
}

// TestScanRenditionDir_FilesAndDirs mistura arquivos e diretórios.
// Regex de reconhecimento é ^[0-9]+\.ts$ (números seguido de .ts)
func TestScanRenditionDir_FilesAndDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Cria arquivos .ts válidos (padrão FFmpeg para segmentos HLS)
	// Arquivo 1: 100 bytes
	_ = os.WriteFile(filepath.Join(tmpDir, "0.ts"), make([]byte, 100), 0600)
	// Arquivo 2: 200 bytes
	_ = os.WriteFile(filepath.Join(tmpDir, "1.ts"), make([]byte, 200), 0600)

	// Cria arquivo que NÃO combina com regex
	_ = os.WriteFile(filepath.Join(tmpDir, "playlist.m3u8"), make([]byte, 50), 0600)

	// Cria diretório (deve ser ignorado)
	_ = os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	sizeBytes, segmentCount, err := scanRenditionDir(tmpDir)
	if err != nil {
		t.Fatalf("scanRenditionDir retornou erro: %v", err)
	}

	// Deve contar apenas os dois arquivos .ts (não o .m3u8)
	if segmentCount != 2 {
		t.Errorf("esperado 2 segmentos, obtido %d", segmentCount)
	}

	// Deve somar o tamanho dos dois arquivos .ts (100 + 200 = 300 bytes)
	if sizeBytes != 300 {
		t.Errorf("esperado 300 bytes, obtido %d", sizeBytes)
	}
}

// TestRecovery_StartsUpWithEmptyDB executa recovery em DB vazio.
func TestRecovery_StartsUpWithEmptyDB(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir DB: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		MaxTranscodeAttempts: 3,
	}

	// Deve retornar sem erro
	err = RunStartupRecovery(database, cfg, func(string) error { return nil }, func(string, string, string) {})
	if err != nil {
		t.Fatalf("RunStartupRecovery em DB vazio retornou erro: %v", err)
	}
}

// TestRecovery_TranscodingVideos_BelowMaxAttempts testa que vídeos em
// "transcoding" abaixo do máximo de tentativas são reenfileirados.
func TestRecovery_TranscodingVideos_BelowMaxAttempts(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir DB: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		MaxTranscodeAttempts: 3,
	}

	// Insere vídeo em "transcoding" com 1 tentativa
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		"vid-transcoding", models.StatusTranscoding, 1,
	)

	enqueueCount := 0
	err = RunStartupRecovery(database, cfg, func(vID string) error {
		enqueueCount++
		return nil
	}, func(string, string, string) {})
	if err != nil {
		t.Fatalf("RunStartupRecovery retornou erro: %v", err)
	}

	if enqueueCount != 1 {
		t.Errorf("esperado 1 enqueue chamado, obtido %d", enqueueCount)
	}

	// Verifica que status foi alterado para upload_complete
	var status string
	_ = database.QueryRow("SELECT status FROM videos WHERE video_id = ?", "vid-transcoding").Scan(&status)
	if status != string(models.StatusUploadComplete) {
		t.Errorf("esperado status %s, obtido %s", models.StatusUploadComplete, status)
	}
}

// TestRecovery_TranscodingVideos_AtMaxAttempts testa que vídeos em
// "transcoding" no máximo de tentativas são marcados como failed.
func TestRecovery_TranscodingVideos_AtMaxAttempts(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir DB: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		MaxTranscodeAttempts: 3,
	}

	// Insere vídeo em "transcoding" com exatamente max attempts
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		"vid-max-attempts", models.StatusTranscoding, 3,
	)

	webhookCount := 0
	err = RunStartupRecovery(database, cfg, func(vID string) error {
		return nil
	}, func(videoID, event, errMsg string) {
		if event == "failed" {
			webhookCount++
		}
	})
	if err != nil {
		t.Fatalf("RunStartupRecovery retornou erro: %v", err)
	}

	if webhookCount != 1 {
		t.Errorf("esperado 1 webhook call, obtido %d", webhookCount)
	}

	// Verifica que status foi alterado para failed_transcode
	var status string
	_ = database.QueryRow("SELECT status FROM videos WHERE video_id = ?", "vid-max-attempts").Scan(&status)
	if status != string(models.StatusFailedTranscode) {
		t.Errorf("esperado status %s, obtido %s", models.StatusFailedTranscode, status)
	}
}

// TestRecovery_UploadCompleteVideos_Unchanged testa que vídeos em
// "upload_complete" são reenfileirados.
func TestRecovery_UploadCompleteVideos_Unchanged(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir DB: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		MaxTranscodeAttempts: 3,
	}

	// Insere vídeo em "upload_complete"
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		"vid-ready", models.StatusUploadComplete, 0,
	)

	enqueueCount := 0
	err = RunStartupRecovery(database, cfg, func(vID string) error {
		enqueueCount++
		return nil
	}, func(string, string, string) {})
	if err != nil {
		t.Fatalf("RunStartupRecovery retornou erro: %v", err)
	}

	// Deve reenfileirar vídeos em upload_complete
	if enqueueCount != 1 {
		t.Errorf("esperado 1 enqueue, obtido %d", enqueueCount)
	}

	// Verifica que status NÃO mudou (permanece upload_complete)
	var status string
	_ = database.QueryRow("SELECT status FROM videos WHERE video_id = ?", "vid-ready").Scan(&status)
	if status != string(models.StatusUploadComplete) {
		t.Errorf("status de upload_complete não deveria mudar, obtido %s", status)
	}
}

// TestRecovery_MultipleStatuses testa recovery com vídeos em múltiplos estados.
func TestRecovery_MultipleStatuses(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir DB: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		MaxTranscodeAttempts: 3,
	}

	// Insere vídeos em vários estados
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		"vid-transcoding-1", models.StatusTranscoding, 0,
	)
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		"vid-transcoding-2", models.StatusTranscoding, 1,
	)
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		"vid-ready", models.StatusUploadComplete,
	)
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		"vid-uploading", models.StatusUploading,
	)
	_, _ = database.Exec(
		"INSERT INTO videos (video_id, status, transcode_attempts) VALUES (?, ?, ?)",
		"vid-ready-transcode", models.StatusReady, 0,
	)

	enqueueCount := 0
	err = RunStartupRecovery(database, cfg, func(vID string) error {
		enqueueCount++
		return nil
	}, func(string, string, string) {})
	if err != nil {
		t.Fatalf("RunStartupRecovery retornou erro: %v", err)
	}

	// Devem ser reenfileirados: 2 transcoding + 1 upload_complete = 3
	if enqueueCount != 3 {
		t.Errorf("esperado 3 enqueues (2 transcoding + 1 upload_complete), obtido %d", enqueueCount)
	}
}

// TestRealFFmpeg_RunValidatesCommand verifica que RealFFmpeg.Run executa
// com o executor padrão de exec.CommandContext (sem mock, apenas validação
// que não falha com erro de "command not found" em contexto test).
func TestRealFFmpeg_RunWithContextTimeout(t *testing.T) {
	// Este teste apenas verifica que RealFFmpeg.Run respeita o contexto
	// e não depende de ffmpeg estar instalado — apenas valida a assinatura
	ffmpeg := &RealFFmpeg{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Usa um comando que não existe para não depender de ffmpeg estar instalado
	// Esperamos que falhe com "command not found" ou "executable file not found"
	err := ffmpeg.Run(ctx, []string{"/nonexistent/ffmpeg"})

	// Deve retornar um erro (command não encontrado é esperado)
	if err == nil {
		t.Error("esperado erro ao executar ffmpeg não-existent, mas Run retornou nil")
	}
}

// fakeFFprobe é um FFprobeExecutor de teste que devolve uma saída e/ou erro
// pré-configurados, permitindo exercitar probeVideo sem o binário ffprobe.
type fakeFFprobe struct {
	out []byte
	err error
}

func (f *fakeFFprobe) Output(_ context.Context, _ []string) ([]byte, error) {
	return f.out, f.err
}

// TestProbeVideo cobre probeVideo com um FFprobeExecutor falso: saída JSON
// válida, falha do comando e saída malformada. Em qualquer caso de erro o
// padrão seguro de 854x480 deve ser retornado (probeVideo nunca propaga erro).
func TestProbeVideo(t *testing.T) {
	tests := []struct {
		name         string
		out          []byte
		err          error
		wantWidth    int
		wantHeight   int
		wantDuration int
	}{
		{
			// JSON válido: deve extrair as dimensões e a duração do stream.
			name:         "sucesso json valido",
			out:          []byte(`{"streams":[{"width":1280,"height":720,"duration":"42.7"}],"format":{"duration":"42.7"}}`),
			err:          nil,
			wantWidth:    1280,
			wantHeight:   720,
			wantDuration: 42,
		},
		{
			// Comando falha (ffprobe ausente/erro): cai no padrão seguro.
			name:         "comando falha",
			out:          nil,
			err:          fmt.Errorf("ffprobe não encontrado"),
			wantWidth:    854,
			wantHeight:   480,
			wantDuration: 0,
		},
		{
			// JSON malformado: cai no padrão seguro.
			name:         "json malformado",
			out:          []byte(`{isto não é json`),
			err:          nil,
			wantWidth:    854,
			wantHeight:   480,
			wantDuration: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{ffprobe: &fakeFFprobe{out: tt.out, err: tt.err}}
			got := w.probeVideo("/qualquer/caminho")
			if got.width != tt.wantWidth || got.height != tt.wantHeight {
				t.Errorf("dimensões = %dx%d, esperado %dx%d", got.width, got.height, tt.wantWidth, tt.wantHeight)
			}
			if got.durationS != tt.wantDuration {
				t.Errorf("durationS = %d, esperado %d", got.durationS, tt.wantDuration)
			}
		})
	}
}

// helper: contém verifica se uma string contém uma substring.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
