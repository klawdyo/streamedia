package transcode

import (
	"fmt"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
)

// TestQueue_EnqueueAndProcess verifica que a fila enfileira e processa
// itens sequencialmente. Usa um TranscodeFunc que registra os video IDs
// processados e um canal para sincronizar a conclusão.
func TestQueue_EnqueueAndProcess(t *testing.T) {
	// Abre banco em memória
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	// Configura a fila com 1 worker
	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	// Variáveis compartilhadas para rastrear processamento
	var mu sync.Mutex
	var processedIDs []string
	done := make(chan struct{})
	var counter int

	// TranscodeFunc que registra IDs processados
	transcodeFunc := func(videoID string) error {
		mu.Lock()
		processedIDs = append(processedIDs, videoID)
		counter++
		if counter == 3 {
			close(done)
		}
		mu.Unlock()
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)
	queue.Start()

	// Enfileira 3 vídeos
	err = queue.Enqueue("v1")
	if err != nil {
		t.Fatalf("Enqueue(v1) falhou: %v", err)
	}
	err = queue.Enqueue("v2")
	if err != nil {
		t.Fatalf("Enqueue(v2) falhou: %v", err)
	}
	err = queue.Enqueue("v3")
	if err != nil {
		t.Fatalf("Enqueue(v3) falhou: %v", err)
	}

	// Aguarda processamento com timeout de 2 segundos
	select {
	case <-done:
		// Processamento concluído
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando processamento dos 3 vídeos")
	}

	queue.Stop()

	// Verifica que todos os 3 foram processados
	mu.Lock()
	if len(processedIDs) != 3 {
		t.Errorf("esperava 3 vídeos processados, obtive %d", len(processedIDs))
	}
	mu.Unlock()
}

// TestQueue_SequentialWithOneWorker verifica que com um único worker
// o processamento é sequencial (time >= 100ms para 2 itens com 50ms cada).
func TestQueue_SequentialWithOneWorker(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	var mu sync.Mutex
	var processedCount int
	done := make(chan struct{})

	// Worker que dorme 50ms
	transcodeFunc := func(videoID string) error {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		processedCount++
		if processedCount == 2 {
			close(done)
		}
		mu.Unlock()
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)

	start := time.Now()
	queue.Start()

	queue.Enqueue("v1")
	queue.Enqueue("v2")

	// Aguarda conclusão
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	elapsed := time.Since(start)
	queue.Stop()

	// Processamento sequencial deve levar >= 100ms (2 items × 50ms cada)
	if elapsed < 100*time.Millisecond {
		t.Errorf("processamento muito rápido: esperava >= 100ms, obtive %v", elapsed)
	}
}

// TestQueue_FullQueueReturnsError verifica que Enqueue retorna erro quando
// a fila está cheia (buffer size = 2, 1 worker que bloqueia).
func TestQueue_FullQueueReturnsError(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	// Fila com buffer = 2
	cfg := &config.Config{
		QueueMaxSize:     2,
		TranscodeWorkers: 1,
	}

	// Canal para desbloquear o worker
	unblockCh := make(chan struct{})

	// Worker que bloqueia até que unblockCh seja sinalizado
	transcodeFunc := func(videoID string) error {
		<-unblockCh
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)
	queue.Start()

	// Enfileira primeiro item (é processado/ocupando o worker)
	err = queue.Enqueue("v1")
	if err != nil {
		t.Fatalf("Enqueue(v1) falhou: %v", err)
	}

	// Dá tempo para o worker pegar o item
	time.Sleep(10 * time.Millisecond)

	// Enfileira 2 itens (preenchem o buffer de 2)
	err = queue.Enqueue("v2")
	if err != nil {
		t.Fatalf("Enqueue(v2) falhou: %v", err)
	}
	err = queue.Enqueue("v3")
	if err != nil {
		t.Fatalf("Enqueue(v3) falhou: %v", err)
	}

	// Tenta enfileirar 4º item — deve retornar erro (fila cheia)
	err = queue.Enqueue("v4")
	if err == nil {
		t.Error("esperava erro ao enfileirar em fila cheia, mas Enqueue() retornou nil")
	}

	// Desbloqueia o worker para limpeza
	close(unblockCh)
	queue.Stop()
}

// TestQueue_LenReturnsCurrentSize verifica que Len() retorna o número de itens
// na fila (buffer) sem chamar Start().
func TestQueue_LenReturnsCurrentSize(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	// Worker que bloqueia até Stop
	blockCh := make(chan struct{})
	transcodeFunc := func(videoID string) error {
		<-blockCh
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)

	// Antes de Start, Len() deve ser 0
	if queue.Len() != 0 {
		t.Errorf("Len() antes de Start: esperava 0, obtive %d", queue.Len())
	}

	// Enfileira 3 itens sem chamar Start (vão para o buffer do canal)
	queue.Enqueue("v1")
	queue.Enqueue("v2")
	queue.Enqueue("v3")

	// Len() deve retornar 3
	if queue.Len() != 3 {
		t.Errorf("Len() após 3 Enqueue: esperava 3, obtive %d", queue.Len())
	}

	// Inicia e para para limpeza
	queue.Start()
	time.Sleep(10 * time.Millisecond)
	close(blockCh)
	queue.Stop()
}

// TestQueue_StopDrainsGracefully verifica que Stop() completa sem pânico
// e drena a fila corretamente dentro de um tempo razoável.
func TestQueue_StopDrainsGracefully(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	var mu sync.Mutex
	var processedCount int

	transcodeFunc := func(videoID string) error {
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		processedCount++
		mu.Unlock()
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)
	queue.Start()

	queue.Enqueue("v1")
	queue.Enqueue("v2")

	// Stop deve completar sem pânico e dentro de tempo razoável
	start := time.Now()
	queue.Stop()
	elapsed := time.Since(start)

	// Deve completar em menos de 1 segundo (com margem para processamento)
	if elapsed > 1*time.Second {
		t.Errorf("Stop() demorou muito: %v", elapsed)
	}

	// Verifica que os 2 itens foram processados
	mu.Lock()
	if processedCount != 2 {
		t.Errorf("esperava 2 itens processados, obtive %d", processedCount)
	}
	mu.Unlock()
}

// TestQueue_WorkerErrorDoesNotCrash verifica que erros no TranscodeFunc
// não fazem a fila parar ou entrar em pânico. Todos os 3 itens devem ser
// processados.
func TestQueue_WorkerErrorDoesNotCrash(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	var mu sync.Mutex
	var processedCount int
	done := make(chan struct{})

	// TranscodeFunc que sempre retorna erro
	transcodeFunc := func(videoID string) error {
		mu.Lock()
		processedCount++
		if processedCount == 3 {
			close(done)
		}
		mu.Unlock()
		return fmt.Errorf("erro proposital para %s", videoID)
	}

	queue := NewQueue(cfg, database, transcodeFunc)
	queue.Start()

	queue.Enqueue("v1")
	queue.Enqueue("v2")
	queue.Enqueue("v3")

	// Aguarda processamento com timeout
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando processamento")
	}

	queue.Stop()

	// Verifica que os 3 itens foram tentados apesar dos erros
	mu.Lock()
	if processedCount != 3 {
		t.Errorf("esperava 3 itens processados, obtive %d", processedCount)
	}
	mu.Unlock()
}

// TestQueue_UpdatesStatusOnStart verifica que Enqueue atualiza o status
// do vídeo no banco para 'transcoding' antes de enfileirar. O worker verifica
// que o status foi atualizado.
func TestQueue_UpdatesStatusOnStart(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	// Insere um vídeo com status 'upload_complete'
	_, err = database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		"test-video-1", "upload_complete",
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	var mu sync.Mutex
	var statusSeen string
	done := make(chan struct{})

	// Worker que verifica o status do vídeo no banco
	transcodeFunc := func(videoID string) error {
		var status string
		err := database.QueryRow(
			"SELECT status FROM videos WHERE video_id = ?",
			videoID,
		).Scan(&status)
		if err != nil {
			t.Errorf("erro ao consultar status: %v", err)
		}
		mu.Lock()
		statusSeen = status
		close(done)
		mu.Unlock()
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)
	queue.Start()

	// Enfileira — deve atualizar status para 'transcoding'
	err = queue.Enqueue("test-video-1")
	if err != nil {
		t.Fatalf("Enqueue() falhou: %v", err)
	}

	// Aguarda o worker ler o status
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	queue.Stop()

	// Verifica que o worker viu o status como 'transcoding'
	mu.Lock()
	if statusSeen != "transcoding" {
		t.Errorf("esperava status 'transcoding', worker viu %q", statusSeen)
	}
	mu.Unlock()
}
