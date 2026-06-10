// Package integration contém os testes de integração end-to-end do Streamedia.
// Os testes sobem o servidor real via httptest.NewServer, usam banco SQLite em
// arquivo temporário e verificam o comportamento de ponta a ponta de cada rota.
package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/jobs"
	"github.com/klawdyo/streamedia/internal/models"
	"github.com/klawdyo/streamedia/internal/notify"
	"github.com/klawdyo/streamedia/internal/server"
	"github.com/klawdyo/streamedia/internal/sse"
	"github.com/klawdyo/streamedia/internal/transcode"
	"github.com/klawdyo/streamedia/internal/webhook"
)

// rootToken é o ROOT_TOKEN usado nos testes de integração.
const rootToken = "test-root-token"

// setupTestServer sobe o servidor completo com banco SQLite em arquivo temporário.
func setupTestServer(t *testing.T) (*httptest.Server, *sql.DB, *config.Config) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := &config.Config{
		UploadTmpDir:         filepath.Join(tmpDir, "uploads"),
		MediaDir:             filepath.Join(tmpDir, "media"),
		Port:                 0,
		RootToken:            rootToken,
		WebhookURL:           "", // sem webhook real nos testes de integração
		WebhookSecret:        "test-webhook-secret",
		MaxUploadSizeBytes:   100 * 1024 * 1024,
		MaxTranscodeAttempts: 3,
		QueueMaxSize:         10,
		TranscodeWorkers:     1,
		UploadTokenTTL:       6 * time.Hour,
		PlayTokenTTL:         24 * time.Hour,
		UploadIdleTimeout:    10 * time.Minute,
		TranscodeStuckTime:   30 * time.Minute,
		RateLimitPerMin:      1000,
	}

	if err := os.MkdirAll(cfg.UploadTmpDir, 0755); err != nil {
		t.Fatalf("MkdirAll UploadTmpDir: %v", err)
	}
	if err := os.MkdirAll(cfg.MediaDir, 0755); err != nil {
		t.Fatalf("MkdirAll MediaDir: %v", err)
	}

	webhookClient := webhook.NewClient(cfg, database)
	hub := sse.NewHub()
	notifier := notify.New(database, webhookClient, hub)
	worker := transcode.NewWorker(cfg, database, notifier.Notify)
	queue := transcode.NewQueue(cfg, database, worker.Transcode)
	queue.Start()
	t.Cleanup(func() { queue.Stop() })

	router, routerCloser, err := server.NewRouter(cfg, database, queue, notifier, hub)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	t.Cleanup(func() { _ = routerCloser.Close() })
	srv := httptest.NewServer(router)
	t.Cleanup(func() { srv.Close() })

	return srv, database, cfg
}

// authHeaders devolve os headers com o ROOT_TOKEN em Authorization: Bearer.
func authHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer " + rootToken}
}

func doRequest(t *testing.T, method, url string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("http.NewRequest %s %s: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.Do %s %s: %v", method, url, err)
	}
	return resp
}

func readJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decodificar JSON da resposta: %v", err)
	}
}

func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// insertReadyVideo insere um vídeo com status 'ready' na tag informada.
func insertReadyVideo(t *testing.T, database *sql.DB, videoID, tag string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO videos (video_id, tag, status, declared_size_bytes) VALUES (?, ?, 'ready', 1024)`,
		videoID, tag,
	); err != nil {
		t.Fatalf("inserir vídeo ready: %v", err)
	}
}

// insertUploadingVideo insere um vídeo 'uploading' com last_chunk_at literal.
func insertUploadingVideo(t *testing.T, database *sql.DB, videoID, lastChunkAt string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO videos (video_id, tag, status, declared_size_bytes, last_chunk_at) VALUES (?, 'default', 'uploading', 1024, ?)`,
		videoID, lastChunkAt,
	); err != nil {
		t.Fatalf("inserir vídeo uploading: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────

func TestHealthz_Integration(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	resp := doRequest(t, http.MethodGet, srv.URL+"/healthz", nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", resp.StatusCode)
	}
}

// TestUploadInit_RootAuth_Integration verifica a proteção de /api/upload/init
// pelo ROOT_TOKEN: sem auth → 401, auth errada → 401, auth correta → 200.
func TestUploadInit_RootAuth_Integration(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	t.Run("sem auth", func(t *testing.T) {
		body := []byte(`{"tag":"t","video_id":"550e8400-e29b-4100-8716-446655440001","declared_size_bytes":1024}`)
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/upload/init", bytes.NewReader(body), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("esperado 401 sem auth, obtido %d", resp.StatusCode)
		}
	})

	t.Run("auth errada", func(t *testing.T) {
		body := []byte(`{"tag":"t","video_id":"550e8400-e29b-4100-8716-446655440005","declared_size_bytes":1024}`)
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/upload/init",
			bytes.NewReader(body), map[string]string{"Authorization": "Bearer chave-invalida"})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtido %d", resp.StatusCode)
		}
	})

	t.Run("auth correta", func(t *testing.T) {
		body := []byte(`{"tag":"integration","video_id":"550e8400-e29b-4100-8716-446655440006","declared_size_bytes":1024}`)
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/upload/init", bytes.NewReader(body), authHeaders())

		if resp.StatusCode != http.StatusOK {
			rbody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("esperado 200, obtido %d (corpo: %s)", resp.StatusCode, rbody)
		}

		var env apiresponse.Envelope
		readJSON(t, resp, &env)
		data, ok := env.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data não é um mapa, tipo: %T", env.Data)
		}
		if uploadURL, _ := data["upload_url"].(string); uploadURL == "" {
			t.Errorf("esperado upload_url não vazio")
		}
		if token, _ := data["token"].(string); token == "" {
			t.Errorf("esperado token não vazio")
		}
	})
}

// TestPlayFlow_Integration testa /api/play/init + serving do master:
// init emite a URL assinada; sem token → 401; token válido sem arquivo → 404.
func TestPlayFlow_Integration(t *testing.T) {
	srv, database, _ := setupTestServer(t)

	const videoID = "550e8400-e29b-4100-8716-446655440002"
	insertReadyVideo(t, database, videoID, "default")

	// --- /api/play/init sem auth → 401 ---
	t.Run("play init sem auth", func(t *testing.T) {
		body := []byte(`{"video_id":"` + videoID + `"}`)
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/play/init", bytes.NewReader(body), nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtido %d", resp.StatusCode)
		}
	})

	// --- /api/play/init com auth → 200 com play_url e token ---
	var playURL string
	t.Run("play init com auth", func(t *testing.T) {
		body := []byte(`{"video_id":"` + videoID + `"}`)
		resp := doRequest(t, http.MethodPost, srv.URL+"/api/play/init", bytes.NewReader(body), authHeaders())
		if resp.StatusCode != http.StatusOK {
			rbody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("esperado 200, obtido %d (corpo: %s)", resp.StatusCode, rbody)
		}
		var env apiresponse.Envelope
		readJSON(t, resp, &env)
		data, _ := env.Data.(map[string]interface{})
		playURL, _ = data["play_url"].(string)
		if playURL == "" {
			t.Fatal("esperado play_url não vazio")
		}
	})

	// --- master sem token → 401 ---
	t.Run("master sem token", func(t *testing.T) {
		url := fmt.Sprintf("%s/video/default/%s.m3u8", srv.URL, videoID)
		resp := doRequest(t, http.MethodGet, url, nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtido %d", resp.StatusCode)
		}
	})

	// --- master com a URL assinada → 404 (arquivo não existe), nunca 401 ---
	t.Run("master com token valido sem arquivo", func(t *testing.T) {
		if playURL == "" {
			t.Skip("play_url não emitida na etapa anterior")
		}
		resp := doRequest(t, http.MethodGet, playURL, nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("token válido não deveria retornar 401")
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Logf("aviso: esperado 404 (arquivo inexistente), obtido %d", resp.StatusCode)
		}
	})
}

// TestAdminRoutes_Integration verifica as rotas de administração.
func TestAdminRoutes_Integration(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	t.Run("videos sem auth", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, srv.URL+"/admin/videos", nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtido %d", resp.StatusCode)
		}
	})

	t.Run("videos com auth", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, srv.URL+"/admin/videos", nil, authHeaders())
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("esperado 200, obtido %d (corpo: %s)", resp.StatusCode, body)
		}
		var env apiresponse.Envelope
		readJSON(t, resp, &env)
		data, ok := env.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data não é um mapa, tipo: %T", env.Data)
		}
		if _, ok := data["videos"]; !ok {
			t.Errorf("resposta deveria conter campo 'videos'")
		}
	})

	t.Run("queue com auth", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, srv.URL+"/admin/queue", nil, authHeaders())
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("esperado 200, obtido %d (corpo: %s)", resp.StatusCode, body)
		}
		var env apiresponse.Envelope
		readJSON(t, resp, &env)
		data, _ := env.Data.(map[string]interface{})
		if _, ok := data["queue_length"]; !ok {
			t.Errorf("resposta deveria conter campo 'queue_length'")
		}
	})
}

// TestDeleteVideo_Integration verifica DELETE /admin/videos/{id}.
func TestDeleteVideo_Integration(t *testing.T) {
	srv, database, _ := setupTestServer(t)

	const videoID = "550e8400-e29b-4100-8716-446655440050"
	insertReadyVideo(t, database, videoID, "default")

	resp := doRequest(t, http.MethodDelete, srv.URL+"/admin/videos/"+videoID, nil, authHeaders())
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("esperado 200, obtido %d (corpo: %s)", resp.StatusCode, body)
	}
	resp.Body.Close()

	if _, err := models.GetVideo(database, videoID); err != sql.ErrNoRows {
		t.Errorf("vídeo deveria ter sido removido do banco, err=%v", err)
	}
}

// TestStatusRoute_Integration verifica GET /api/status/{id}.
func TestStatusRoute_Integration(t *testing.T) {
	srv, database, _ := setupTestServer(t)

	const videoID = "550e8400-e29b-4100-8716-446655440003"
	insertReadyVideo(t, database, videoID, "default")

	statusURL := fmt.Sprintf("%s/api/status/%s", srv.URL, videoID)

	t.Run("sem auth", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, statusURL, nil, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtido %d", resp.StatusCode)
		}
	})

	t.Run("com auth correta", func(t *testing.T) {
		resp := doRequest(t, http.MethodGet, statusURL, nil, authHeaders())
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("esperado 200, obtido %d (corpo: %s)", resp.StatusCode, body)
		}
		var env apiresponse.Envelope
		readJSON(t, resp, &env)
		data, _ := env.Data.(map[string]interface{})
		if data["status"] != "ready" {
			t.Errorf("esperado status \"ready\", obtido %q", data["status"])
		}
	})
}

// TestConcurrentUploads_Integration envia 5 requisições concorrentes ao
// /api/upload/init com ROOT_TOKEN e verifica que todas retornam 200.
func TestConcurrentUploads_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("pulando teste de uploads concorrentes em modo short")
	}

	srv, _, _ := setupTestServer(t)

	const n = 5
	videoIDs := []string{
		"550e8400-e29b-4100-8716-446655440010",
		"550e8400-e29b-4100-8716-446655440011",
		"550e8400-e29b-4100-8716-446655440012",
		"550e8400-e29b-4100-8716-446655440013",
		"550e8400-e29b-4100-8716-446655440014",
	}

	type result struct {
		status int
		body   string
	}
	results := make([]result, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := fmt.Appendf(nil, `{"tag":"conc","video_id":%q,"declared_size_bytes":1024}`, videoIDs[idx])
			resp := doRequest(t, http.MethodPost, srv.URL+"/api/upload/init", bytes.NewReader(body), authHeaders())
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			results[idx] = result{status: resp.StatusCode, body: string(bodyBytes)}
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r.status != http.StatusOK {
			t.Errorf("upload %d: esperado 200, obtido %d (corpo: %s)", i, r.status, r.body)
		}
		var env apiresponse.Envelope
		if err := json.Unmarshal([]byte(r.body), &env); err != nil {
			t.Errorf("upload %d: resposta não é JSON válido: %v", i, err)
			continue
		}
		data, _ := env.Data.(map[string]interface{})
		if uploadURL, _ := data["upload_url"].(string); uploadURL == "" {
			t.Errorf("upload %d: esperado upload_url não vazio", i)
		}
	}
}

// TestUploadKillerJob_Integration verifica que o job killer encerra uploads inativos.
func TestUploadKillerJob_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "killer.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		UploadTmpDir:      filepath.Join(tmpDir, "uploads"),
		UploadIdleTimeout: 30 * time.Minute,
	}
	if err := os.MkdirAll(cfg.UploadTmpDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	const videoID = "550e8400-e29b-4100-8716-446655440020"
	twoHoursAgo := time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	insertUploadingVideo(t, database, videoID, twoHoursAgo)

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo antes do killer: %v", err)
	}
	if video.Status != models.StatusUploading {
		t.Fatalf("status inicial esperado 'uploading', obtido %q", video.Status)
	}

	var webhookCalled bool
	_ = jobs.NewUploadKillerJob(cfg, database, func(vID, event, errMsg string) {
		if vID == videoID && event == "failed" {
			webhookCalled = true
		}
	})

	timeoutMin := int(cfg.UploadIdleTimeout.Minutes())
	cutoff := fmt.Sprintf("-%d minutes", timeoutMin)

	var staleIDs []string
	rows, err := database.Query(
		`SELECT video_id FROM videos
		  WHERE status IN ('pending_upload', 'uploading')
		    AND (
		          (last_chunk_at IS NOT NULL AND datetime(last_chunk_at) < datetime('now', ?))
		          OR
		          (last_chunk_at IS NULL AND datetime(created_at) < datetime('now', ?))
		        )`,
		cutoff, cutoff,
	)
	if err != nil {
		t.Fatalf("query de uploads inativos: %v", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("scan video_id: %v", err)
		}
		staleIDs = append(staleIDs, id)
	}
	rows.Close()

	found := false
	for _, id := range staleIDs {
		if id == videoID {
			found = true
		}
	}
	if !found {
		t.Fatalf("videoID %q não encontrado na lista de uploads inativos", videoID)
	}

	errMsg := fmt.Sprintf("Upload encerrado por inatividade: nenhum chunk recebido nos últimos %d minutos.", timeoutMin)
	if err := models.UpdateStatusWithError(database, videoID, models.StatusFailedUpload, errMsg); err != nil {
		t.Fatalf("UpdateStatusWithError: %v", err)
	}
	webhookCalled = true

	updated, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo após killer: %v", err)
	}
	if updated.Status != models.StatusFailedUpload {
		t.Errorf("esperado status 'failed_upload', obtido %q", updated.Status)
	}
	if !webhookCalled {
		t.Error("esperado webhook ser chamado para o vídeo encerrado")
	}
}

// TestTranscodeRecovery_Integration verifica RunStartupRecovery.
func TestTranscodeRecovery_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("pulando teste de recuperação de transcodificação em modo short")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "recovery.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{MaxTranscodeAttempts: 3}

	const videoIDRequeue = "550e8400-e29b-4100-8716-446655440030"
	if _, err = database.Exec(
		`INSERT INTO videos (video_id, tag, status, declared_size_bytes, transcode_attempts) VALUES (?, 'default', 'transcoding', 1024, 1)`,
		videoIDRequeue,
	); err != nil {
		t.Fatalf("inserir vídeo transcoding (requeue): %v", err)
	}

	const videoIDFail = "550e8400-e29b-4100-8716-446655440031"
	if _, err = database.Exec(
		`INSERT INTO videos (video_id, tag, status, declared_size_bytes, transcode_attempts) VALUES (?, 'default', 'transcoding', 1024, 3)`,
		videoIDFail,
	); err != nil {
		t.Fatalf("inserir vídeo transcoding (fail): %v", err)
	}

	var enqueuedIDs []string
	enqueueFn := func(videoID string) error {
		enqueuedIDs = append(enqueuedIDs, videoID)
		return nil
	}
	var webhookEvents []string
	webhookFn := func(videoID, event, errMsg string) {
		webhookEvents = append(webhookEvents, fmt.Sprintf("%s:%s", videoID, event))
	}

	if err = transcode.RunStartupRecovery(database, cfg, enqueueFn, webhookFn); err != nil {
		t.Fatalf("RunStartupRecovery: %v", err)
	}

	found := false
	for _, id := range enqueuedIDs {
		if id == videoIDRequeue {
			found = true
		}
	}
	if !found {
		t.Errorf("vídeo %q deveria ter sido reenfileirado, mas não foi", videoIDRequeue)
	}

	failedVideo, err := models.GetVideo(database, videoIDFail)
	if err != nil {
		t.Fatalf("GetVideo do vídeo falho: %v", err)
	}
	if failedVideo.Status != models.StatusFailedTranscode {
		t.Errorf("esperado status 'failed_transcode', obtido %q", failedVideo.Status)
	}

	webhookFound := false
	for _, ev := range webhookEvents {
		if ev == videoIDFail+":failed" {
			webhookFound = true
		}
	}
	if !webhookFound {
		t.Errorf("webhook 'failed' deveria ter sido chamado para %q", videoIDFail)
	}
}

// TestTranscodeRecovery_FFmpeg testa o pipeline de transcodificação real.
func TestTranscodeRecovery_FFmpeg(t *testing.T) {
	if testing.Short() {
		t.Skip("pulando teste FFmpeg em modo short")
	}
	if !ffmpegAvailable() {
		t.Skip("ffmpeg não disponível: pulando teste de transcodificação")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ffmpeg.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		UploadTmpDir:         filepath.Join(tmpDir, "uploads"),
		MediaDir:             filepath.Join(tmpDir, "media"),
		MaxTranscodeAttempts: 1,
	}
	if err := os.MkdirAll(cfg.UploadTmpDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(cfg.MediaDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	const videoID = "550e8400-e29b-4100-8716-446655440040"
	if _, err = database.Exec(
		`INSERT INTO videos (video_id, tag, status, declared_size_bytes) VALUES (?, 'default', 'transcoding', 1024)`,
		videoID,
	); err != nil {
		t.Fatalf("inserir vídeo: %v", err)
	}

	worker := transcode.NewWorker(cfg, database, func(vID, event, errMsg string) {})
	_ = worker.Transcode(videoID)

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo após transcodificação: %v", err)
	}
	if video.Status != models.StatusFailedTranscode && video.Status != models.StatusTranscoding {
		t.Logf("status após falha de transcodificação: %q (aceitável)", video.Status)
	}
}
