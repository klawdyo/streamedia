package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/klawdyo/streamedia/internal/apiresponse"
)

// evictionTTL é o tempo de inatividade após o qual uma entrada é removida.
const evictionTTL = 10 * time.Minute

// evictionInterval é a frequência do goroutine de limpeza.
const evictionInterval = 1 * time.Minute

// limiterEntry armazena o rate.Limiter e o timestamp do último acesso.
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64 // unix timestamp (segundos)
}

// RateLimiter implementa um middleware de rate limiting por IP.
type RateLimiter struct {
	limiters sync.Map  // string IP → *limiterEntry
	rate     rate.Limit
	burst    int
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewRateLimiter cria um novo RateLimiter com o limite especificado (requisições por minuto).
// Inicia um goroutine de limpeza que remove entries inativas a cada minuto.
func NewRateLimiter(perMin int) *RateLimiter {
	rl := &RateLimiter{
		rate:   rate.Limit(float64(perMin) / 60.0),
		burst:  perMin,
		stopCh: make(chan struct{}),
	}
	rl.wg.Add(1)
	go rl.evictLoop()
	return rl
}

// Stop encerra o goroutine de limpeza e aguarda sua finalização.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
	rl.wg.Wait()
}

// evictLoop remove periodicamente entries inativas do mapa.
func (rl *RateLimiter) evictLoop() {
	defer rl.wg.Done()
	ticker := time.NewTicker(evictionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-evictionTTL).Unix()
			rl.limiters.Range(func(key, value any) bool {
				entry := value.(*limiterEntry)
				if entry.lastSeen.Load() < cutoff {
					rl.limiters.Delete(key)
				}
				return true
			})
		case <-rl.stopCh:
			return
		}
	}
}

// getLimiter obtém ou cria um rate.Limiter para o IP especificado.
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now().Unix()
	entry := &limiterEntry{limiter: rate.NewLimiter(rl.rate, rl.burst)}
	entry.lastSeen.Store(now)

	actual, loaded := rl.limiters.LoadOrStore(ip, entry)
	e := actual.(*limiterEntry)
	if loaded {
		// Entry já existia: atualiza lastSeen.
		e.lastSeen.Store(now)
	}
	return e.limiter
}

// extractIP extrai o endereço IP da requisição.
// Prioridade:
// 1. X-Real-IP
// 2. X-Forwarded-For (primeiro valor)
// 3. RemoteAddr (com remoção da porta)
func extractIP(r *http.Request) string {
	// Tenta X-Real-IP
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Tenta X-Forwarded-For
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Pega o primeiro IP da lista (separado por vírgula)
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			if ip := strings.TrimSpace(ips[0]); ip != "" {
				return ip
			}
		}
	}

	// Extrai IP de RemoteAddr removendo a porta
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}

	// Fallback: retorna RemoteAddr como está
	return r.RemoteAddr
}

// Middleware retorna um middleware HTTP que aplica rate limiting por IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			// Rate limit excedido — responde com o envelope padrão da API.
			// O header Retry-After deve ser setado ANTES de chamar
			// apiresponse.Error, porque esta já chama WriteHeader.
			w.Header().Set("Retry-After", "60")
			apiresponse.Error(w, http.StatusTooManyRequests, "Muitas requisições. Tente novamente em 60 segundos.")
			return
		}

		// Continua para o próximo handler
		next.ServeHTTP(w, r)
	})
}
