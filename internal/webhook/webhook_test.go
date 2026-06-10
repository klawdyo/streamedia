package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// openTestDB abre um banco de dados em memória para testes.
func openTestDB(t *testing.T) *sql.DB {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco de dados em memória: %v", err)
	}

	return database
}

// TestSend_SuccessOnFirstAttempt verifica se o webhook é enviado com sucesso na primeira tentativa.
func TestSend_SuccessOnFirstAttempt(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	// Cria um vídeo de teste
	videoID := "test-video-1"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	// Recupera o vídeo
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	// Cria um servidor mock que responde 200
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Cria o cliente com o servidor mock
	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	// Envia o webhook
	err = client.Send(videoID, "processing", video)
	if err != nil {
		t.Fatalf("Send falhou: %v", err)
	}

	// Verifica se exatamente 1 requisição foi feita
	if requestCount != 1 {
		t.Errorf("esperava 1 requisição, obteve %d", requestCount)
	}

	// Verifica o webhook_log
	logs, err := GetWebhookLog(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter webhook_log: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("esperava 1 entrada em webhook_log, obteve %d", len(logs))
	}
	if !logs[0].Success {
		t.Errorf("esperava success=true, obteve false")
	}
}

// TestSend_NoURLSkips garante que, sem WEBHOOK_URL configurada, Send é um no-op
// (não envia, não registra log, não retorna erro) — o webhook fica desabilitado.
func TestSend_NoURLSkips(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-nourl"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	// Cliente sem WEBHOOK_URL → resolveURL devolve ok=false.
	client := NewClient(&config.Config{WebhookURL: "", WebhookSecret: ""}, database)
	if err := client.Send(videoID, "processing", video); err != nil {
		t.Fatalf("Send sem URL deveria devolver nil, obteve: %v", err)
	}

	logs, err := GetWebhookLog(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter webhook_log: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("esperava 0 entradas em webhook_log (envio pulado), obteve %d", len(logs))
	}
}

// TestSend_SignatureVerification verifica se o header X-Signature é enviado corretamente.
func TestSend_SignatureVerification(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-2"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	webhookSecret := "test-secret-123"
	var capturedSignature string
	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSignature = r.Header.Get("X-Signature")
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		capturedBody = buf[:n]
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: webhookSecret,
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "processing", video)
	if err != nil {
		t.Fatalf("Send falhou: %v", err)
	}

	// Verifica se o header X-Signature começa com "sha256="
	if len(capturedSignature) == 0 {
		t.Fatalf("X-Signature vazio")
	}
	if len(capturedSignature) < 7 || capturedSignature[:7] != "sha256=" {
		t.Errorf("X-Signature não começa com 'sha256=': %s", capturedSignature)
	}

	// Recomputa o HMAC-SHA256 e compara
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(capturedBody)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	receivedSignature := capturedSignature[7:] // Remove "sha256="
	if receivedSignature != expectedSignature {
		t.Errorf("assinatura incorreta. esperava %s, obteve %s", expectedSignature, receivedSignature)
	}
}

// TestSend_RetryOnFailure verifica se o cliente tenta novamente após falhas.
func TestSend_RetryOnFailure(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-3"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	// Servidor que responde 500 nas primeiras 2 vezes, 200 na 3ª
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "processing", video)
	if err != nil {
		t.Fatalf("Send falhou: %v", err)
	}

	// Verifica se 3 requisições foram feitas
	if requestCount != 3 {
		t.Errorf("esperava 3 requisições, obteve %d", requestCount)
	}

	// Verifica que a última entrada em webhook_log tem success=true
	logs, err := GetWebhookLog(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter webhook_log: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("esperava 3 entradas em webhook_log, obteve %d", len(logs))
	}
	if !logs[len(logs)-1].Success {
		t.Errorf("esperava última entrada com success=true, obteve false")
	}
}

// TestSend_AllAttemptsFailure verifica o comportamento quando todas as tentativas falham.
func TestSend_AllAttemptsFailure(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-4"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	// Servidor sempre retorna erro
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "processing", video)
	if err == nil {
		t.Fatalf("esperava erro, mas Send retornou nil")
	}

	// Verifica se 3 requisições foram feitas
	if requestCount != 3 {
		t.Errorf("esperava 3 requisições, obteve %d", requestCount)
	}

	// Verifica webhook_log: deve ter 3 entradas, todas com success=false
	logs, err := GetWebhookLog(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter webhook_log: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("esperava 3 entradas em webhook_log, obteve %d", len(logs))
	}
	for i, log := range logs {
		if log.Success {
			t.Errorf("entrada %d tem success=true, esperava false", i)
		}
	}
}

// TestSend_Timeout verifica o comportamento quando o servidor não responde a tempo.
func TestSend_Timeout(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-5"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	// Servidor que dorme mais de 10 segundos
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "processing", video)
	if err == nil {
		t.Fatalf("esperava erro por timeout, mas Send retornou nil")
	}
}

// TestPayload_Processing verifica o payload para evento "processing".
func TestPayload_Processing(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-6"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	var capturedPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		capturedPayload = buf[:n]
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "processing", video)
	if err != nil {
		t.Fatalf("Send falhou: %v", err)
	}

	// Desserializa o payload capturado
	var payload WebhookPayload
	err = json.Unmarshal(capturedPayload, &payload)
	if err != nil {
		t.Fatalf("erro ao desserializar payload: %v", err)
	}

	// Verifica que duration_s é nil
	if payload.DurationS != nil {
		t.Errorf("esperava duration_s=nil, obteve %v", *payload.DurationS)
	}

	// Verifica que resolutions é um slice vazio (ou nil)
	if payload.Resolutions != nil && len(payload.Resolutions) > 0 {
		t.Errorf("esperava resolutions vazio, obteve %v", payload.Resolutions)
	}

	// Verifica que error_message é nil
	if payload.ErrorMessage != nil {
		t.Errorf("esperava error_message=nil, obteve %v", *payload.ErrorMessage)
	}
}

// TestPayload_Ready verifica o payload para um vídeo com duração e resolutions.
func TestPayload_Ready(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-7"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	// Segue a máquina de estados: pending_upload → uploading → upload_complete → transcoding → ready
	if err := models.UpdateStatus(database, videoID, models.StatusUploading); err != nil {
		t.Fatalf("erro ao atualizar para uploading: %v", err)
	}
	if err := models.UpdateStatus(database, videoID, models.StatusUploadComplete); err != nil {
		t.Fatalf("erro ao atualizar para upload_complete: %v", err)
	}
	if err := models.UpdateStatus(database, videoID, models.StatusTranscoding); err != nil {
		t.Fatalf("erro ao atualizar para transcoding: %v", err)
	}

	// Atualiza o vídeo para status ready com duração e resolutions
	if err := models.SetReady(database, videoID, 47, []int{480, 720}); err != nil {
		t.Fatalf("erro ao atualizar vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	var capturedPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		capturedPayload = buf[:n]
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "ready", video)
	if err != nil {
		t.Fatalf("Send falhou: %v", err)
	}

	// Desserializa o payload capturado
	var payload WebhookPayload
	err = json.Unmarshal(capturedPayload, &payload)
	if err != nil {
		t.Fatalf("erro ao desserializar payload: %v", err)
	}

	// Verifica duration_s
	if payload.DurationS == nil || *payload.DurationS != 47 {
		t.Errorf("esperava duration_s=47, obteve %v", payload.DurationS)
	}

	// Verifica resolutions
	if len(payload.Resolutions) != 2 || payload.Resolutions[0] != 480 || payload.Resolutions[1] != 720 {
		t.Errorf("esperava resolutions=[480, 720], obteve %v", payload.Resolutions)
	}
}

// TestPayload_Failed verifica o payload para um vídeo com erro.
func TestPayload_Failed(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-8"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	// Segue a máquina de estados até transcoding e depois marca como falhou
	if err := models.UpdateStatus(database, videoID, models.StatusUploading); err != nil {
		t.Fatalf("erro ao atualizar para uploading: %v", err)
	}
	if err := models.UpdateStatus(database, videoID, models.StatusUploadComplete); err != nil {
		t.Fatalf("erro ao atualizar para upload_complete: %v", err)
	}
	if err := models.UpdateStatus(database, videoID, models.StatusTranscoding); err != nil {
		t.Fatalf("erro ao atualizar para transcoding: %v", err)
	}

	// Atualiza o vídeo com erro
	if err := models.UpdateStatusWithError(database, videoID, models.StatusFailedTranscode, "Erro na transcodificação"); err != nil {
		t.Fatalf("erro ao atualizar vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	var capturedPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		capturedPayload = buf[:n]
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "failed", video)
	if err != nil {
		t.Fatalf("Send falhou: %v", err)
	}

	// Desserializa o payload capturado
	var payload WebhookPayload
	err = json.Unmarshal(capturedPayload, &payload)
	if err != nil {
		t.Fatalf("erro ao desserializar payload: %v", err)
	}

	// Verifica error_message
	if payload.ErrorMessage == nil || *payload.ErrorMessage != "Erro na transcodificação" {
		t.Errorf("esperava error_message='Erro na transcodificação', obteve %v", payload.ErrorMessage)
	}
}

// TestWebhookLog_RecordsAttempts verifica se o webhook_log registra todas as tentativas corretamente.
func TestWebhookLog_RecordsAttempts(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()

	videoID := "test-video-9"
	if err := models.InsertVideo(database, videoID, 1000); err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter vídeo: %v", err)
	}

	// Servidor que responde 500 nas primeiras 2 vezes, 200 na 3ª
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookURL:    server.URL,
		WebhookSecret: "test-secret",
	}
	client := NewClient(cfg, database)

	err = client.Send(videoID, "processing", video)
	if err != nil {
		t.Fatalf("Send falhou: %v", err)
	}

	// Consulta webhook_log
	logs, err := GetWebhookLog(database, videoID)
	if err != nil {
		t.Fatalf("erro ao obter webhook_log: %v", err)
	}

	// Verifica que há 3 entradas
	if len(logs) != 3 {
		t.Errorf("esperava 3 entradas em webhook_log, obteve %d", len(logs))
		return
	}

	// Verifica que as primeiras 2 têm success=false
	if logs[0].Success || logs[1].Success {
		t.Errorf("primeiras 2 entradas deveriam ter success=false")
	}

	// Verifica que a última tem success=true
	if !logs[2].Success {
		t.Errorf("última entrada deveria ter success=true")
	}

	// Verifica que todos pertencem ao mesmo videoID
	for i, log := range logs {
		if log.VideoID != videoID {
			t.Errorf("entrada %d tem videoID=%s, esperava %s", i, log.VideoID, videoID)
		}
	}
}
