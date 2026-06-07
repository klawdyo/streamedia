package transcode

import (
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// TestUploadStoresUnderProjectDirectory verifica que, ao transcodificar um
// vídeo associado a um projeto, o worker grava a saída HLS sob
// <MEDIA_DIR>/<slug-do-projeto>/<video_id>/... — issue #6, T34. O diretório
// é resolvido via models.ResolveVideoRootDir a partir do project_id do vídeo.
func TestUploadStoresUnderProjectDirectory(t *testing.T) {
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

	// Cria um projeto — RootDir é o slug, conforme models.CreateProject.
	project, _, err := models.CreateProject(database, "Project Storage Co")
	if err != nil {
		t.Fatalf("erro ao criar projeto: %v", err)
	}

	const videoID = "test-project-storage"
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, status, project_id) VALUES (?, ?, ?)",
		videoID, "upload_complete", project.ID,
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	inputFile := filepath.Join(uploadTmpDir, videoID)
	if err := os.WriteFile(inputFile, []byte("fake video data"), 0644); err != nil {
		t.Fatalf("erro ao criar arquivo de entrada: %v", err)
	}

	cfg := &config.Config{
		UploadTmpDir:         uploadTmpDir,
		MediaDir:             mediaDir,
		MaxTranscodeAttempts: 3,
		KeepOriginal:         false,
	}

	w := NewWorker(cfg, database, func(videoID, event, errMsg string) {})

	expectedOutputDir := filepath.Join(mediaDir, project.RootDir, videoID)
	mockExec := &mockFFmpeg{
		err: nil,
		createFiles: func(args []string) {
			outputDir := filepath.Join(expectedOutputDir, "480")
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				t.Logf("aviso: erro ao criar outputDir: %v", err)
			}
			_ = os.WriteFile(filepath.Join(outputDir, "playlist.m3u8"), []byte("#EXTM3U\n"), 0644)
			_ = os.WriteFile(filepath.Join(outputDir, "0.ts"), []byte("fake ts"), 0644)
		},
	}
	setWorkerExecutor(w, mockExec)

	if err := w.Transcode(videoID); err != nil {
		t.Fatalf("Transcode retornou erro: %v", err)
	}

	// Verifica que o master.m3u8 foi escrito sob <MEDIA_DIR>/<slug>/<video_id>/.
	masterPath := filepath.Join(expectedOutputDir, "master.m3u8")
	if _, err := os.Stat(masterPath); err != nil {
		t.Errorf("esperava master.m3u8 em %s (diretório isolado por projeto), mas: %v", masterPath, err)
	}

	// Verifica que NADA foi gravado diretamente sob <MEDIA_DIR>/<video_id>/
	// (layout legado) — a saída deve estar isolada sob o diretório do projeto.
	legacyMasterPath := filepath.Join(mediaDir, videoID, "master.m3u8")
	if _, err := os.Stat(legacyMasterPath); err == nil {
		t.Errorf("não esperava master.m3u8 no caminho legado %s quando o vídeo tem projeto associado", legacyMasterPath)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao recuperar vídeo: %v", err)
	}
	if video.Status != models.StatusReady {
		t.Errorf("esperava status 'ready', obtive %s", video.Status)
	}
}

// TestUploadStoresUnderLegacyDirectory verifica que vídeos sem projeto
// associado (project_id NULL) continuam sendo gravados diretamente sob
// <MEDIA_DIR>/<video_id>/... — preserva o layout legado para vídeos ainda
// não migrados (issue #6, T34; ResolveVideoRootDir resolve "" para nil).
func TestUploadStoresUnderLegacyDirectory(t *testing.T) {
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

	const videoID = "test-legacy-storage"
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		videoID, "upload_complete",
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	inputFile := filepath.Join(uploadTmpDir, videoID)
	if err := os.WriteFile(inputFile, []byte("fake video data"), 0644); err != nil {
		t.Fatalf("erro ao criar arquivo de entrada: %v", err)
	}

	cfg := &config.Config{
		UploadTmpDir:         uploadTmpDir,
		MediaDir:             mediaDir,
		MaxTranscodeAttempts: 3,
		KeepOriginal:         false,
	}

	w := NewWorker(cfg, database, func(videoID, event, errMsg string) {})

	expectedOutputDir := filepath.Join(mediaDir, videoID)
	mockExec := &mockFFmpeg{
		err: nil,
		createFiles: func(args []string) {
			outputDir := filepath.Join(expectedOutputDir, "480")
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				t.Logf("aviso: erro ao criar outputDir: %v", err)
			}
			_ = os.WriteFile(filepath.Join(outputDir, "playlist.m3u8"), []byte("#EXTM3U\n"), 0644)
			_ = os.WriteFile(filepath.Join(outputDir, "0.ts"), []byte("fake ts"), 0644)
		},
	}
	setWorkerExecutor(w, mockExec)

	if err := w.Transcode(videoID); err != nil {
		t.Fatalf("Transcode retornou erro: %v", err)
	}

	masterPath := filepath.Join(expectedOutputDir, "master.m3u8")
	if _, err := os.Stat(masterPath); err != nil {
		t.Errorf("esperava master.m3u8 em %s (layout legado), mas: %v", masterPath, err)
	}
}
