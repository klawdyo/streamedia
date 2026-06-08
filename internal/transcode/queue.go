package transcode

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// TranscodeFunc é o tipo do worker de transcodificação.
// Recebe o ID do vídeo e retorna um erro em caso de falha.
type TranscodeFunc func(videoID string) error

// Queue gerencia a fila de transcodificação com pool de workers.
type Queue struct {
	ch     chan string       // canal bufferizado com os IDs de vídeo pendentes
	cfg    *config.Config    // configuração (tamanho da fila, número de workers)
	db     *sql.DB           // conexão com o banco para atualizar status
	worker TranscodeFunc     // função executada por worker para cada vídeo
	wg     sync.WaitGroup    // aguarda término de todos os workers
	stopCh chan struct{}     // sinaliza encerramento dos workers
	once   sync.Once         // garante que stopCh seja fechado apenas uma vez
}

// NewQueue cria uma nova fila com canal bufferizado de tamanho cfg.QueueMaxSize.
// NÃO inicia os workers — chame Start() para isso.
func NewQueue(cfg *config.Config, db *sql.DB, worker TranscodeFunc) *Queue {
	return &Queue{
		// Canal bufferizado: comporta até QueueMaxSize itens sem bloquear o envio.
		ch:     make(chan string, cfg.QueueMaxSize),
		cfg:    cfg,
		db:     db,
		worker: worker,
		// Canal de parada usado para sinalizar encerramento aos workers.
		stopCh: make(chan struct{}),
	}
}

// Start inicia cfg.TranscodeWorkers goroutines de processamento.
// Cada goroutine consome IDs do canal e os processa via worker.
func (q *Queue) Start() {
	for i := 0; i < q.cfg.TranscodeWorkers; i++ {
		// Registra a goroutine no WaitGroup antes de iniciá-la.
		q.wg.Add(1)
		go func() {
			defer q.wg.Done()
			for {
				select {
				case videoID := <-q.ch:
					// Erros do worker são ignorados de propósito:
					// o próprio worker atualiza o status no banco.
					q.worker(videoID)
				case <-q.stopCh:
					// Ao receber o sinal de parada, drena os itens restantes
					// do buffer antes de encerrar, garantindo que nada fique
					// pendente quando Stop() retornar.
					for {
						select {
						case videoID := <-q.ch:
							q.worker(videoID)
						default:
							return
						}
					}
				}
			}
		}()
	}
}

// Stop encerra os workers de forma graciosa.
// Fecha stopCh apenas uma vez (sync.Once) e aguarda o término de todos
// os workers, incluindo a drenagem dos itens restantes na fila.
func (q *Queue) Stop() {
	// Fecha o canal de parada uma única vez, mesmo em chamadas concorrentes.
	q.once.Do(func() {
		close(q.stopCh)
	})
	// Aguarda todos os workers concluírem (item em andamento + drenagem).
	q.wg.Wait()
}

// Enqueue atualiza o status do vídeo para 'transcoding' e o enfileira.
// Retorna erro se a atualização do banco falhar ou se a fila estiver cheia.
func (q *Queue) Enqueue(videoID string) error {
	// Atualiza o status para 'transcoding' via máquina de estados (T58:
	// substitui UPDATE direto que bypassava validTransitions e usava formato
	// de timestamp inconsistente). Se a transição for inválida ou o banco
	// falhar, retorna erro — o vídeo NÃO é enfileirado.
	if err := models.UpdateStatus(q.db, videoID, models.StatusTranscoding); err != nil {
		return fmt.Errorf("erro ao atualizar status para transcoding: %w", err)
	}

	// Envio não bloqueante: se o buffer estiver cheio, retorna erro.
	select {
	case q.ch <- videoID:
		return nil
	default:
		return fmt.Errorf("Fila de transcodificação está cheia.")
	}
}

// Len retorna o número de itens atualmente no buffer da fila.
func (q *Queue) Len() int {
	return len(q.ch)
}
