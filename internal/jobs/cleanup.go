package jobs

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/klawdyo/streamedia/internal/models"
)

// cleanupTickInterval define a frequência com que o job varre o banco em
// busca de tokens expirados e os remove.
const cleanupTickInterval = 24 * time.Hour

// TokenCleanupJob é o job periódico que remove tokens de upload expirados
// do banco de dados para evitar acúmulo de lixo.
type TokenCleanupJob struct {
	db     *sql.DB
	ticker *time.Ticker
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewTokenCleanupJob cria uma nova instância do job de limpeza de tokens.
func NewTokenCleanupJob(db *sql.DB) *TokenCleanupJob {
	return &TokenCleanupJob{
		db:     db,
		ticker: time.NewTicker(cleanupTickInterval),
		stopCh: make(chan struct{}),
	}
}

// Start inicia a goroutine que executa o job a cada intervalo do ticker.
func (j *TokenCleanupJob) Start() {
	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		for {
			select {
			case <-j.ticker.C:
				// Ignora o erro: o próximo tick tentará novamente.
				_, _ = j.runOnce()
			case <-j.stopCh:
				j.ticker.Stop()
				return
			}
		}
	}()
}

// Stop encerra a goroutine do job e aguarda sua finalização.
func (j *TokenCleanupJob) Stop() {
	close(j.stopCh)
	j.wg.Wait()
}

// runOnce executa uma única varredura: deleta todos os tokens com expires_at
// no passado e retorna o número de tokens deletados.
// É a unidade testável da lógica do job.
func (j *TokenCleanupJob) runOnce() (int64, error) {
	count, err := models.DeleteExpiredTokens(j.db)
	if err != nil {
		log.Printf("erro ao deletar tokens expirados: %v\n", err)
		return 0, err
	}
	if count > 0 {
		log.Printf("tokens expirados deletados: %d\n", count)
	}
	return count, nil
}
