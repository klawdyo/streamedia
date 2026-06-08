package transcode

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
)

// insertQueueTestVideo insere um vídeo no banco com status upload_complete
// (estado válido para transição → transcoding via Enqueue). Necessário
// desde T58, quando Enqueue passou a validar transições via state machine.
func insertQueueTestVideo(t *testing.T, database *sql.DB, videoID string) {
	t.Helper()
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, 'upload_complete')",
		videoID,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo de teste %s: %v", videoID, err)
	}
}

// sliceEqual compara dois slices de inteiros.
func sliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// containsArg verifica se args contém um valor que inclui a substring s.
func containsArg(args []string, s string) bool {
	for _, arg := range args {
		if strings.Contains(arg, s) {
			return true
		}
	}
	return false
}

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

	// Insere vídeos no banco com status upload_complete (T58: Enqueue valida transição)
	insertQueueTestVideo(t, database, "v1")
	insertQueueTestVideo(t, database, "v2")
	insertQueueTestVideo(t, database, "v3")

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

	insertQueueTestVideo(t, database, "v1")
	insertQueueTestVideo(t, database, "v2")

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

	insertQueueTestVideo(t, database, "v1")
	insertQueueTestVideo(t, database, "v2")
	insertQueueTestVideo(t, database, "v3")
	insertQueueTestVideo(t, database, "v4")

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

	insertQueueTestVideo(t, database, "v1")
	insertQueueTestVideo(t, database, "v2")
	insertQueueTestVideo(t, database, "v3")

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

	insertQueueTestVideo(t, database, "v1")
	insertQueueTestVideo(t, database, "v2")

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

	insertQueueTestVideo(t, database, "v1")
	insertQueueTestVideo(t, database, "v2")
	insertQueueTestVideo(t, database, "v3")

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

// TestQueue_ConcurrentEnqueue testa enfileiramento concorrente de múltiplos
// vídeos sem race condition.
func TestQueue_ConcurrentEnqueue(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     100,
		TranscodeWorkers: 2,
	}

	var mu sync.Mutex
	var processedCount int
	done := make(chan struct{})

	transcodeFunc := func(videoID string) error {
		mu.Lock()
		processedCount++
		if processedCount == 10 {
			close(done)
		}
		mu.Unlock()
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)
	queue.Start()

	// Insere 10 vídeos no banco antes de enfileirar concorrentemente
	for i := 1; i <= 10; i++ {
		insertQueueTestVideo(t, database, fmt.Sprintf("v%d", i))
	}

	// Enfileira 10 vídeos concorrentemente
	for i := 1; i <= 10; i++ {
		go func(idx int) {
			videoID := fmt.Sprintf("v%d", idx)
			if err := queue.Enqueue(videoID); err != nil {
				t.Errorf("Enqueue(%s) falhou: %v", videoID, err)
			}
		}(i)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout aguardando 10 vídeos processados")
	}

	queue.Stop()

	mu.Lock()
	if processedCount != 10 {
		t.Errorf("esperava 10 processados, obtive %d", processedCount)
	}
	mu.Unlock()
}

// TestQueue_MultipleworkersParallel testa que múltiplos workers processam
// itens em paralelo (tempo total menor que sequencial).
func TestQueue_MultipleWorkersParallel(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 3,
	}

	var mu sync.Mutex
	var processedCount int
	done := make(chan struct{})

	// Worker que dorme 30ms
	transcodeFunc := func(videoID string) error {
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		processedCount++
		if processedCount == 6 {
			close(done)
		}
		mu.Unlock()
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)

	// Insere 6 vídeos no banco
	for i := 1; i <= 6; i++ {
		insertQueueTestVideo(t, database, fmt.Sprintf("v%d", i))
	}

	start := time.Now()
	queue.Start()

	// Enfileira 6 vídeos
	for i := 1; i <= 6; i++ {
		queue.Enqueue(fmt.Sprintf("v%d", i))
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	elapsed := time.Since(start)
	queue.Stop()

	// 6 itens × 30ms com 3 workers paralelos deve levar ~60ms (6/3 = 2 batches)
	// Aceitamos até 200ms considerando overhead
	if elapsed > 200*time.Millisecond {
		t.Errorf("processamento com 3 workers demorou muito: %v", elapsed)
	}
}

// TestQueue_StopBeforeStart verifica que Stop() funciona mesmo se chamado
// antes de Start() (sem pânico, mesmo com sync.Once).
func TestQueue_StopBeforeStart(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	transcodeFunc := func(videoID string) error {
		return nil
	}

	queue := NewQueue(cfg, database, transcodeFunc)

	// Não chama Start(), apenas Stop()
	queue.Stop()

	// Se não houve pânico, o teste passa
}

// TestQueue_EnqueueAfterStop verifica que Enqueue não trava se chamado
// após Stop() (comportamento undefined, mas não deve travar).
func TestQueue_EnqueueAfterStop(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	queue := NewQueue(cfg, database, func(videoID string) error {
		return nil
	})
	queue.Start()
	queue.Stop()

	insertQueueTestVideo(t, database, "v1")

	// Tenta enfileirar após parada (pode falhar ou não, mas não deve travar)
	queue.Enqueue("v1")
	// Se não travou, o teste passa
}

// TestEnqueue_DBErrorReturnsError verifica que Enqueue retorna erro quando
// a atualização de status no banco falha, e NÃO enfileira o vídeo no canal.
func TestEnqueue_DBErrorReturnsError(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco: %v", err)
	}

	// Insere um vídeo para que a query tenha o que atualizar.
	_, err = database.Exec(
		"INSERT INTO videos (video_id, status) VALUES (?, ?)",
		"v-db-error", "upload_complete",
	)
	if err != nil {
		database.Close()
		t.Fatalf("erro ao inserir vídeo: %v", err)
	}

	cfg := &config.Config{
		QueueMaxSize:     50,
		TranscodeWorkers: 1,
	}

	queue := NewQueue(cfg, database, func(videoID string) error {
		return nil
	})

	// Fecha o banco para forçar erro no Exec dentro de Enqueue.
	database.Close()

	// Enqueue deve retornar erro (DB fechado).
	err = queue.Enqueue("v-db-error")
	if err == nil {
		t.Error("esperava erro ao enfileirar com banco fechado, mas Enqueue() retornou nil")
	}

	// A fila NÃO deve conter o item (não foi enfileirado).
	if queue.Len() != 0 {
		t.Errorf("fila deveria estar vazia após erro de DB, mas Len()=%d", queue.Len())
	}
}
