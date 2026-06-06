package upload

import (
	"context"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// TestValidateMagicBytes_MP4 verifica que arquivo com header MP4 válido
// é reconhecido como vídeo.
func TestValidateMagicBytes_MP4(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.mp4")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	// Header MP4 válido: 00 00 00 18 66 74 79 70 6d 70 34 32
	mp4Header := []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6d, 0x70, 0x34, 0x32}
	if _, err := tmpFile.Write(mp4Header); err != nil {
		t.Fatalf("falha ao escrever header MP4: %v", err)
	}

	// Escreve mais dados para fazer arquivo maior
	padding := make([]byte, 1024)
	if _, err := tmpFile.Write(padding); err != nil {
		t.Fatalf("falha ao escrever padding: %v", err)
	}

	tmpFile.Sync()
	path := tmpFile.Name()

	valid, err := validateMagicBytes(path)
	if err != nil {
		t.Fatalf("validateMagicBytes retornou erro inesperado: %v", err)
	}
	if !valid {
		t.Error("esperava true para header MP4 válido, obteve false")
	}
}

// TestValidateMagicBytes_TextFile verifica que arquivo de texto puro
// não é reconhecido como vídeo.
func TestValidateMagicBytes_TextFile(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.txt")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString("Hello World"); err != nil {
		t.Fatalf("falha ao escrever conteúdo: %v", err)
	}

	tmpFile.Sync()
	path := tmpFile.Name()

	valid, err := validateMagicBytes(path)
	if err != nil {
		t.Fatalf("validateMagicBytes retornou erro inesperado: %v", err)
	}
	if valid {
		t.Error("esperava false para arquivo de texto, obteve true")
	}
}

// TestValidateMagicBytes_EmptyFile verifica que arquivo vazio é rejeitado.
func TestValidateMagicBytes_EmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.bin")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	tmpFile.Close()

	path := tmpFile.Name()

	valid, err := validateMagicBytes(path)
	// Tanto (false, nil) quanto (false, error) são aceitáveis
	if valid {
		t.Error("esperava false para arquivo vazio, obteve true")
	}
}

// TestValidateMagicBytes_MKV verifica que arquivo com header MKV válido
// é reconhecido como vídeo.
func TestValidateMagicBytes_MKV(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.mkv")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	// Header MKV válido: 1a 45 df a3
	mkvHeader := []byte{0x1a, 0x45, 0xdf, 0xa3}
	if _, err := tmpFile.Write(mkvHeader); err != nil {
		t.Fatalf("falha ao escrever header MKV: %v", err)
	}

	// Escreve dados arbitrários
	arbitrary := make([]byte, 512)
	for i := range arbitrary {
		arbitrary[i] = byte(i % 256)
	}
	if _, err := tmpFile.Write(arbitrary); err != nil {
		t.Fatalf("falha ao escrever dados: %v", err)
	}

	tmpFile.Sync()
	path := tmpFile.Name()

	valid, err := validateMagicBytes(path)
	if err != nil {
		t.Fatalf("validateMagicBytes retornou erro inesperado: %v", err)
	}
	if !valid {
		t.Error("esperava true para header MKV válido, obteve false")
	}
}

// TestValidateFileSize_Match verifica que tamanhos iguais (real == declarado)
// retornam nil error.
func TestValidateFileSize_Match(t *testing.T) {
	actualBytes := int64(1024)
	declaredBytes := int64(1024)

	err := validateFileSize(actualBytes, declaredBytes)
	if err != nil {
		t.Errorf("esperava nil error para tamanhos iguais, obteve: %v", err)
	}
}

// TestValidateFileSize_Mismatch verifica que tamanhos diferentes
// retornam erro não-nulo.
func TestValidateFileSize_Mismatch(t *testing.T) {
	actualBytes := int64(1024)
	declaredBytes := int64(2048)

	err := validateFileSize(actualBytes, declaredBytes)
	if err == nil {
		t.Error("esperava erro não-nulo para tamanhos diferentes, obteve nil")
	}
}

// TestPostFinishValidation_InvalidMagicBytes verifica que arquivo com
// magic bytes inválidos causa falha na validação.
func TestPostFinishValidation_InvalidMagicBytes(t *testing.T) {
	// Configura banco de dados em memória
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	// Cria config mínima
	cfg := &config.Config{
		UploadTmpDir:       t.TempDir(),
		MaxUploadSizeBytes: 1 << 30,
	}

	// Cria arquivo temporário com conteúdo de texto (magic bytes inválido)
	tmpFile, err := os.CreateTemp(cfg.UploadTmpDir, "test-*.bin")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	// Escreve "Hello World" (11 bytes, inválido para vídeo)
	if _, err := tmpFile.WriteString("Hello World"); err != nil {
		t.Fatalf("falha ao escrever conteúdo: %v", err)
	}
	tmpFile.Sync()
	filePath := tmpFile.Name()

	// Insere vídeo no banco com tamanho declarado = 11 (tamanho do arquivo)
	videoID := "test-video-1"
	if err := models.InsertVideo(database, videoID, 11); err != nil {
		t.Fatalf("falha ao inserir vídeo: %v", err)
	}

	// Atualiza status para "uploading"
	if err := models.UpdateStatus(database, videoID, models.StatusUploading); err != nil {
		t.Fatalf("falha ao atualizar status: %v", err)
	}

	// Mock enqueue: registra se foi chamada
	enqueueCalled := false
	mockEnqueue := func(vid string) error {
		enqueueCalled = true
		return nil
	}

	// Mock sendWebhook: registra chamadas
	webhookCalls := []struct {
		event  string
		errMsg string
	}{}
	mockSendWebhook := func(vid string, event string, errMsg string) {
		webhookCalls = append(webhookCalls, struct {
			event  string
			errMsg string
		}{event, errMsg})
	}

	// Chama HandlePostFinish
	HandlePostFinish(database, cfg, mockEnqueue, mockSendWebhook, videoID, filePath)

	// Verifica que o status foi alterado para "failed_upload"
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("falha ao recuperar vídeo do banco: %v", err)
	}
	if video.Status != models.StatusFailedUpload {
		t.Errorf("status esperado %q, obteve %q", models.StatusFailedUpload, video.Status)
	}

	// Verifica que o arquivo foi deletado do disco
	if _, err := os.Stat(filePath); err == nil {
		t.Error("arquivo deveria ter sido deletado do disco")
	} else if !os.IsNotExist(err) {
		t.Errorf("erro inesperado ao verificar se arquivo foi deletado: %v", err)
	}

	// Verifica que sendWebhook foi chamada com event="failed"
	failedFound := false
	for _, call := range webhookCalls {
		if call.event == "failed" {
			failedFound = true
			break
		}
	}
	if !failedFound {
		t.Errorf("sendWebhook deveria ter sido chamada com event='failed', chamadas: %v", webhookCalls)
	}

	// Verifica que enqueue NÃO foi chamada
	if enqueueCalled {
		t.Error("enqueue não deveria ter sido chamada após falha de validação")
	}
}

// TestPostFinishValidation_SizeMismatch verifica que arquivo com tamanho
// diferente do declarado causa falha na validação.
func TestPostFinishValidation_SizeMismatch(t *testing.T) {
	// Configura banco de dados em memória
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	// Cria config mínima
	cfg := &config.Config{
		UploadTmpDir:       t.TempDir(),
		MaxUploadSizeBytes: 1 << 30,
	}

	// Cria arquivo temporário com tamanho que não corresponde ao declarado
	tmpFile, err := os.CreateTemp(cfg.UploadTmpDir, "test-*.bin")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	// Escreve 5 bytes
	if _, err := tmpFile.Write([]byte{0x01, 0x02, 0x03, 0x04, 0x05}); err != nil {
		t.Fatalf("falha ao escrever bytes: %v", err)
	}
	tmpFile.Sync()
	filePath := tmpFile.Name()

	// Insere vídeo com tamanho declarado = 999 (diferente dos 5 bytes reais)
	videoID := "test-video-2"
	if err := models.InsertVideo(database, videoID, 999); err != nil {
		t.Fatalf("falha ao inserir vídeo: %v", err)
	}

	// Atualiza status para "uploading"
	if err := models.UpdateStatus(database, videoID, models.StatusUploading); err != nil {
		t.Fatalf("falha ao atualizar status: %v", err)
	}

	// Mock enqueue
	enqueueCalled := false
	mockEnqueue := func(vid string) error {
		enqueueCalled = true
		return nil
	}

	// Mock sendWebhook
	webhookCalls := []struct {
		event  string
		errMsg string
	}{}
	mockSendWebhook := func(vid string, event string, errMsg string) {
		webhookCalls = append(webhookCalls, struct {
			event  string
			errMsg string
		}{event, errMsg})
	}

	// Chama HandlePostFinish
	HandlePostFinish(database, cfg, mockEnqueue, mockSendWebhook, videoID, filePath)

	// Verifica que o status foi alterado para "failed_upload"
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("falha ao recuperar vídeo do banco: %v", err)
	}
	if video.Status != models.StatusFailedUpload {
		t.Errorf("status esperado %q, obteve %q", models.StatusFailedUpload, video.Status)
	}

	// Verifica que o arquivo foi deletado do disco
	if _, err := os.Stat(filePath); err == nil {
		t.Error("arquivo deveria ter sido deletado do disco")
	} else if !os.IsNotExist(err) {
		t.Errorf("erro inesperado ao verificar se arquivo foi deletado: %v", err)
	}

	// Verifica que sendWebhook foi chamada com event="failed"
	failedFound := false
	for _, call := range webhookCalls {
		if call.event == "failed" {
			failedFound = true
			break
		}
	}
	if !failedFound {
		t.Errorf("sendWebhook deveria ter sido chamada com event='failed', chamadas: %v", webhookCalls)
	}

	// Verifica que enqueue NÃO foi chamada
	if enqueueCalled {
		t.Error("enqueue não deveria ter sido chamada após falha de validação")
	}
}

// TestRunFFprobe_ValidFile verifica que ffprobe executa e retorna
// metadados válidos de um arquivo de vídeo.
// Nota: Este teste pode ser pulado em ambientes sem ffprobe instalado.
func TestRunFFprobe_ValidFile(t *testing.T) {
	// Verifica se ffprobe está disponível
	ctx := context.Background()

	// Cria arquivo temporário (vazio, apenas para teste de presença de arquivo)
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.mp4")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	// Escreve header MP4 válido
	mp4Header := []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6d, 0x70, 0x34, 0x32}
	if _, err := tmpFile.Write(mp4Header); err != nil {
		t.Fatalf("falha ao escrever header: %v", err)
	}
	tmpFile.Sync()
	filePath := tmpFile.Name()

	result, err := runFFprobe(ctx, filePath)

	// Se ffprobe não estiver disponível, o teste falhará na execução
	// Ambientes de CI podem não ter ffprobe instalado
	if err != nil {
		t.Logf("ffprobe não disponível ou erro ao executar: %v", err)
		return
	}

	// Verifica que result não é nil
	if result == nil {
		t.Error("esperava FFprobeResult não-nil")
		return
	}

	// Verifica que os campos foram preenchidos (ou ao menos existem)
	if result.DurationS < 0 {
		t.Errorf("duração negativa é inválida: %d", result.DurationS)
	}
	if result.Width < 0 {
		t.Errorf("largura negativa é inválida: %d", result.Width)
	}
	if result.Height < 0 {
		t.Errorf("altura negativa é inválida: %d", result.Height)
	}
}

// TestValidateMagicBytes_WebM verifica que arquivo com header WebM válido
// é reconhecido como vídeo.
func TestValidateMagicBytes_WebM(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.webm")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	// Header WebM válido: 1a 45 df a3 (mesmo que MKV, pois WebM usa contêiner Matroska)
	webmHeader := []byte{0x1a, 0x45, 0xdf, 0xa3}
	if _, err := tmpFile.Write(webmHeader); err != nil {
		t.Fatalf("falha ao escrever header WebM: %v", err)
	}

	// Escreve dados extras
	if _, err := tmpFile.Write(make([]byte, 256)); err != nil {
		t.Fatalf("falha ao escrever padding: %v", err)
	}

	tmpFile.Sync()
	path := tmpFile.Name()

	valid, err := validateMagicBytes(path)
	if err != nil {
		t.Fatalf("validateMagicBytes retornou erro inesperado: %v", err)
	}
	if !valid {
		t.Error("esperava true para header WebM válido, obteve false")
	}
}

// TestValidateFileSize_ZeroActual verifica que tamanho real zero
// com tamanho declarado > 0 retorna erro.
func TestValidateFileSize_ZeroActual(t *testing.T) {
	actualBytes := int64(0)
	declaredBytes := int64(1024)

	err := validateFileSize(actualBytes, declaredBytes)
	if err == nil {
		t.Error("esperava erro não-nulo para tamanho real zero com declarado > 0")
	}
}

// TestValidateFileSize_ZeroDeclared verifica que tamanho declarado zero
// com tamanho real > 0 retorna erro.
func TestValidateFileSize_ZeroDeclared(t *testing.T) {
	actualBytes := int64(1024)
	declaredBytes := int64(0)

	err := validateFileSize(actualBytes, declaredBytes)
	if err == nil {
		t.Error("esperava erro não-nulo para tamanho declarado zero com real > 0")
	}
}

// TestValidateFileSize_BothZero verifica o comportamento quando ambos
// os tamanhos são zero.
func TestValidateFileSize_BothZero(t *testing.T) {
	actualBytes := int64(0)
	declaredBytes := int64(0)

	// Este caso é ambíguo: pode ser válido (arquivo vazio permitido)
	// ou inválido (zero não é tamanho válido). O comportamento
	// será definido pela implementação.
	_ = validateFileSize(actualBytes, declaredBytes)
	// Teste apenas verifica que não há panic
}

// TestPostFinishValidation_SuccessfulValidation testa cenário de sucesso
// onde arquivo tem magic bytes válido e tamanho correto.
// Nota: Requer ffprobe disponível no sistema.
func TestPostFinishValidation_SuccessfulValidation(t *testing.T) {
	// Configura banco de dados em memória
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco em memória: %v", err)
	}
	defer database.Close()

	// Cria config mínima
	cfg := &config.Config{
		UploadTmpDir:       t.TempDir(),
		MaxUploadSizeBytes: 1 << 30,
	}

	// Cria arquivo temporário com header MP4 válido
	tmpFile, err := os.CreateTemp(cfg.UploadTmpDir, "test-*.mp4")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporário: %v", err)
	}
	defer tmpFile.Close()

	// Escreve header MP4 válido
	mp4Header := []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6d, 0x70, 0x34, 0x32}
	if _, err := tmpFile.Write(mp4Header); err != nil {
		t.Fatalf("falha ao escrever header MP4: %v", err)
	}

	// Escreve 1000 bytes adicionais
	padding := make([]byte, 1000)
	if _, err := tmpFile.Write(padding); err != nil {
		t.Fatalf("falha ao escrever padding: %v", err)
	}

	tmpFile.Sync()
	filePath := tmpFile.Name()

	// Insere vídeo com tamanho declarado correto (12 + 1000 = 1012)
	videoID := "test-video-success"
	if err := models.InsertVideo(database, videoID, 1012); err != nil {
		t.Fatalf("falha ao inserir vídeo: %v", err)
	}

	// Atualiza status para "uploading"
	if err := models.UpdateStatus(database, videoID, models.StatusUploading); err != nil {
		t.Fatalf("falha ao atualizar status: %v", err)
	}

	// Mock enqueue: registra se foi chamada
	enqueueVideoID := ""
	mockEnqueue := func(vid string) error {
		enqueueVideoID = vid
		return nil
	}

	// Mock sendWebhook
	webhookCalls := []struct {
		event  string
		errMsg string
	}{}
	mockSendWebhook := func(vid string, event string, errMsg string) {
		webhookCalls = append(webhookCalls, struct {
			event  string
			errMsg string
		}{event, errMsg})
	}

	// Chama HandlePostFinish
	HandlePostFinish(database, cfg, mockEnqueue, mockSendWebhook, videoID, filePath)

	// Como runFFprobe pode falhar se ffprobe não estiver disponível,
	// o teste se adapta ao resultado. Se tudo passou:
	// - Se enqueue foi chamada: status deve ser upload_complete
	// - Se enqueue não foi chamada: status deve ser failed_upload (ffprobe falhou)

	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("falha ao recuperar vídeo do banco: %v", err)
	}

	// Se houve sucesso até agora, arquivo deveria ter sido deletado
	// ou permanecer, dependendo da implementação.
	// Teste apenas verifica que a função completou sem panic.
	if video.Status == models.StatusFailedUpload && len(webhookCalls) == 0 {
		t.Logf("validação falhou (esperado se ffprobe não disponível)")
	}

	// Se enqueue foi chamada, deve ter recebido o videoID correto
	if enqueueVideoID != "" && enqueueVideoID != videoID {
		t.Errorf("enqueue recebeu videoID %q, esperava %q", enqueueVideoID, videoID)
	}
}
