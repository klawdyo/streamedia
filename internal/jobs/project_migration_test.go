package jobs

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// insertVideoNoProject insere um vídeo "ready" sem project_id (layout
// legado) — usado para testar a migração para o projeto Legacy.
func insertVideoNoProject(t *testing.T, database *sql.DB, videoID string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO videos (video_id, status) VALUES (?, ?)`, videoID, "ready",
	); err != nil {
		t.Fatalf("erro ao inserir vídeo %s: %v", videoID, err)
	}
}

// TestLegacyVideoMigration verifica que MigrateLegacyVideos (issue #6, T34):
//  1. cria o projeto "Legacy" (slug "legacy") na primeira execução;
//  2. move o diretório do vídeo de <MEDIA_DIR>/<video_id> para
//     <MEDIA_DIR>/legacy/<video_id> e associa o vídeo ao projeto;
//  3. é idempotente — uma segunda execução não duplica o projeto nem
//     corrompe o estado (vídeo já migrado não é processado novamente).
func TestLegacyVideoMigration(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	mediaDir := t.TempDir()
	const videoID = "legacy-video-001"
	insertVideoNoProject(t, database, videoID)

	// Cria o diretório de armazenamento no layout antigo (sem prefixo de projeto).
	oldDir := filepath.Join(mediaDir, videoID)
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatalf("erro ao criar diretório legado: %v", err)
	}
	masterPath := filepath.Join(oldDir, "master.m3u8")
	const masterContent = "#EXTM3U\n#EXT-X-VERSION:3\n"
	if err := os.WriteFile(masterPath, []byte(masterContent), 0644); err != nil {
		t.Fatalf("erro ao escrever master.m3u8: %v", err)
	}

	// --- Primeira execução: deve migrar o vídeo. ---
	migrated, err := MigrateLegacyVideos(database, mediaDir)
	if err != nil {
		t.Fatalf("MigrateLegacyVideos retornou erro: %v", err)
	}
	if migrated != 1 {
		t.Errorf("esperava 1 vídeo migrado, obteve %d", migrated)
	}

	// O projeto "Legacy" deve existir com o slug "legacy".
	legacy, err := models.GetProjectBySlug(database, "legacy")
	if err != nil {
		t.Fatalf("esperava projeto 'Legacy' criado (slug 'legacy'), mas: %v", err)
	}
	if legacy.Name != legacyProjectName {
		t.Errorf("esperava nome do projeto %q, obteve %q", legacyProjectName, legacy.Name)
	}

	// O diretório deve ter sido movido para <MEDIA_DIR>/legacy/<video_id>/.
	newDir := filepath.Join(mediaDir, legacy.RootDir, videoID)
	newMasterPath := filepath.Join(newDir, "master.m3u8")
	content, err := os.ReadFile(newMasterPath)
	if err != nil {
		t.Fatalf("esperava master.m3u8 movido para %s, mas: %v", newMasterPath, err)
	}
	if string(content) != masterContent {
		t.Errorf("conteúdo do master.m3u8 movido não confere: obteve %q", string(content))
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("esperava que o diretório legado %s não existisse mais após a migração", oldDir)
	}

	// O vídeo deve estar associado ao projeto Legacy no banco.
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao recuperar vídeo: %v", err)
	}
	if video.ProjectID == nil || *video.ProjectID != legacy.ID {
		t.Errorf("esperava vídeo associado ao projeto Legacy (id=%d), obteve project_id=%v", legacy.ID, video.ProjectID)
	}

	// --- Segunda execução: idempotente, nada a migrar. ---
	migratedAgain, err := MigrateLegacyVideos(database, mediaDir)
	if err != nil {
		t.Fatalf("segunda execução de MigrateLegacyVideos retornou erro: %v", err)
	}
	if migratedAgain != 0 {
		t.Errorf("esperava 0 vídeos migrados na segunda execução (idempotência), obteve %d", migratedAgain)
	}

	// O projeto Legacy não deve ter sido duplicado.
	legacyAgain, err := models.GetProjectBySlug(database, "legacy")
	if err != nil {
		t.Fatalf("erro ao buscar projeto Legacy na segunda verificação: %v", err)
	}
	if legacyAgain.ID != legacy.ID {
		t.Errorf("esperava o mesmo projeto Legacy (id=%d), obteve id=%d — possível duplicação", legacy.ID, legacyAgain.ID)
	}

	// O conteúdo migrado continua intacto e no novo local.
	if _, err := os.Stat(newMasterPath); err != nil {
		t.Errorf("esperava que o arquivo migrado continuasse em %s, mas: %v", newMasterPath, err)
	}
}

// TestLegacyVideoMigration_NoVideosToMigrate verifica que, quando não há
// vídeos sem projeto, MigrateLegacyVideos não migra nada nem retorna erro
// (a criação do projeto "Legacy" é feita de forma eager e idempotente em
// getOrCreateLegacyProject — ele sempre existe ao final, mas nenhum vídeo é
// tocado quando não há candidatos).
func TestLegacyVideoMigration_NoVideosToMigrate(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	mediaDir := t.TempDir()

	// Cria um projeto e um vídeo já associado a ele (nada a migrar).
	project, _, err := models.CreateProject(database, "Already Scoped")
	if err != nil {
		t.Fatalf("erro ao criar projeto: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO videos (video_id, status, project_id) VALUES (?, ?, ?)`,
		"already-scoped-video", "ready", project.ID,
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	migrated, err := MigrateLegacyVideos(database, mediaDir)
	if err != nil {
		t.Fatalf("MigrateLegacyVideos retornou erro: %v", err)
	}
	if migrated != 0 {
		t.Errorf("esperava 0 vídeos migrados, obteve %d", migrated)
	}

	// O vídeo já associado permanece intocado (não remigrado, sem duplicar projeto).
	video, err := models.GetVideo(database, "already-scoped-video")
	if err != nil {
		t.Fatalf("erro ao recuperar vídeo: %v", err)
	}
	if video.ProjectID == nil || *video.ProjectID != project.ID {
		t.Errorf("esperava que o vídeo permanecesse associado ao projeto original (id=%d), obteve project_id=%v", project.ID, video.ProjectID)
	}
}
