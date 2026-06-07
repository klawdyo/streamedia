package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"

	"github.com/klawdyo/streamedia/internal/apiresponse"
)

// RateLimiter implementa um middleware de rate limiting por IP.
type RateLimiter struct {
	limiters sync.Map  // string IP → *rate.Limiter
	rate     rate.Limit
	burst    int
}

// NewRateLimiter cria um novo RateLimiter com o limite especificado (requisições por minuto).
func NewRateLimiter(perMin int) *RateLimiter {
	return &RateLimiter{
		rate:  rate.Limit(float64(perMin) / 60.0),
		burst: perMin,
	}
}

// getLimiter obtém ou cria um rate.Limiter para o IP especificado.
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	limiter, _ := rl.limiters.LoadOrStore(ip, rate.NewLimiter(rl.rate, rl.burst))
	return limiter.(*rate.Limiter)
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
