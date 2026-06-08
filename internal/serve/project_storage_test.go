package serve

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/models"
)

// thirdVideoID é um terceiro UUID v4 válido fixo, distinto de testVideoID e
// secondVideoID, usado para testar o serving de vídeos associados a projetos
// (issue #6, T34: layout de armazenamento isolado por projeto).
const thirdVideoID = "c2aa1e44-5b3f-4a7e-9c1d-2f6b0a8d9e10"

// TestServingResolvesProjectDirectory verifica que MasterHandler e
// StaticHandler servem os arquivos de um vídeo associado a um projeto a
// partir de <MEDIA_DIR>/<slug-do-projeto>/<video_id>/... — issue #6, T34.
// Usa models.ResolveVideoRootDir (via os handlers) para resolver o diretório
// a partir do project_id gravado no vídeo.
func TestServingResolvesProjectDirectory(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	// Cria um projeto — RootDir é o slug, conforme models.CreateProject.
	project, _, err := models.CreateProject(database, "Acme Studios")
	if err != nil {
		t.Fatalf("erro ao criar projeto: %v", err)
	}

	// Insere um vídeo "ready" associado ao projeto.
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, status, project_id) VALUES (?, ?, ?)",
		thirdVideoID, "ready", project.ID,
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	// Escreve os arquivos no layout esperado: <MEDIA_DIR>/<slug>/<video_id>/...
	const masterContent = "#EXTM3U\n#EXT-X-VERSION:3\n480/playlist.m3u8\n"
	const playlistContent = "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:6.0,\n0.ts\n#EXT-X-ENDLIST\n"
	const segmentContent = "fake ts segment data"

	masterPath := filepath.Join(cfg.MediaDir, project.RootDir, thirdVideoID, "master.m3u8")
	playlistPath := filepath.Join(cfg.MediaDir, project.RootDir, thirdVideoID, "480", "playlist.m3u8")
	segmentPath := filepath.Join(cfg.MediaDir, project.RootDir, thirdVideoID, "480", "0.ts")

	writeFile(t, masterPath, masterContent)
	writeFile(t, playlistPath, playlistContent)
	writeFile(t, segmentPath, segmentContent)

	// --- MasterHandler: deve servir o master.m3u8 do diretório do projeto ---
	expires := time.Now().Add(time.Hour).Unix()
	token := auth.GeneratePlayToken(testSecret, thirdVideoID, expires)

	masterHandler := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+thirdVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	rec := httptest.NewRecorder()
	masterHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("MasterHandler: esperava 200, obteve %d (corpo: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != masterContent {
		t.Errorf("MasterHandler: corpo inesperado, esperava servir o master.m3u8 do diretório do projeto %q", project.RootDir)
	}

	// --- StaticHandler: deve servir playlist e segmento do diretório do projeto ---
	staticHandler := NewStaticHandler(cfg, database)

	reqPlaylist := httptest.NewRequest(http.MethodGet, "/videos/"+thirdVideoID+"/480/playlist.m3u8", nil)
	recPlaylist := httptest.NewRecorder()
	staticHandler.ServeHTTP(recPlaylist, reqPlaylist)
	if recPlaylist.Code != http.StatusOK {
		t.Fatalf("StaticHandler (playlist): esperava 200, obteve %d", recPlaylist.Code)
	}
	if recPlaylist.Body.String() != playlistContent {
		t.Errorf("StaticHandler (playlist): corpo inesperado, esperava servir do diretório do projeto %q", project.RootDir)
	}

	reqSegment := httptest.NewRequest(http.MethodGet, "/videos/"+thirdVideoID+"/480/0.ts", nil)
	recSegment := httptest.NewRecorder()
	staticHandler.ServeHTTP(recSegment, reqSegment)
	if recSegment.Code != http.StatusOK {
		t.Fatalf("StaticHandler (segmento): esperava 200, obteve %d", recSegment.Code)
	}
	if recSegment.Body.String() != segmentContent {
		t.Errorf("StaticHandler (segmento): corpo inesperado, esperava servir do diretório do projeto %q", project.RootDir)
	}
}

// TestServingUsesDefaultProjectDirectory verifica que vídeos criados com o
// projeto padrão (EnsureDefaultProject) — substituindo o antigo layout legado
// (project_id NULL, removido na issue #10, T48) — são servidos a partir de
// <MEDIA_DIR>/<slug-do-projeto-padrao>/<video_id>/...
func TestServingUsesDefaultProjectDirectory(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)

	project, err := models.EnsureDefaultProject(database)
	if err != nil {
		t.Fatalf("EnsureDefaultProject: %v", err)
	}

	// Insere um vídeo "ready" associado ao projeto padrão (EnsureDefaultProject).
	if _, err := database.Exec(
		"INSERT INTO videos (video_id, status, project_id) VALUES (?, ?, ?)",
		thirdVideoID, "ready", project.ID,
	); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	const masterContent = "#EXTM3U\n#EXT-X-VERSION:3\n480/playlist.m3u8\n"
	masterPath := filepath.Join(cfg.MediaDir, project.RootDir, thirdVideoID, "master.m3u8")
	writeFile(t, masterPath, masterContent)

	expires := time.Now().Add(time.Hour).Unix()
	token := auth.GeneratePlayToken(testSecret, thirdVideoID, expires)

	masterHandler := NewMasterHandler(cfg, database)
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+thirdVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+token, nil)
	rec := httptest.NewRecorder()
	masterHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao servir vídeo do projeto padrão, obteve %d (corpo: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != masterContent {
		t.Errorf("corpo inesperado: esperava servir o master.m3u8 do diretório <MEDIA_DIR>/<slug-do-projeto-padrao>/<video_id>/")
	}
}
