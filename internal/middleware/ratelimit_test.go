package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRateLimit_AllowsUnderLimit verifica se requisições dentro do limite são permitidas.
func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	limiter := NewRateLimiter(5) // 5 requisições por minuto
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "1.2.3.4"
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip + ":5678"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}
}

// TestRateLimit_BlocksOverLimit verifica se requisições acima do limite são bloqueadas.
func TestRateLimit_BlocksOverLimit(t *testing.T) {
	limiter := NewRateLimiter(3) // 3 requisições por minuto
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "1.2.3.4"
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip + ":5678"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if i < 3 {
			// Primeiras 3 requisições devem passar
			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
			}
		} else {
			// 4ª requisição deve ser bloqueada
			if w.Code != http.StatusTooManyRequests {
				t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusTooManyRequests, w.Code)
			}
		}
	}
}

// TestRateLimit_DifferentIPsAreIndependent verifica se IPs diferentes têm limites independentes.
func TestRateLimit_DifferentIPsAreIndependent(t *testing.T) {
	limiter := NewRateLimiter(2) // 2 requisições por minuto
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP-A faz 2 requisições (atinge o limite)
	ipA := "10.0.0.1"
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ipA + ":1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("IP-A request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// IP-B faz 2 requisições (deve passar — IP diferente)
	ipB := "10.0.0.2"
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ipB + ":1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("IP-B request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// IP-A tenta uma 3ª requisição (deve ser bloqueada)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ipA + ":1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("IP-A request 3: expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}

// TestRateLimit_HeaderRetryAfter verifica se a resposta bloqueada contém o header Retry-After.
func TestRateLimit_HeaderRetryAfter(t *testing.T) {
	limiter := NewRateLimiter(1) // 1 requisição por minuto
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "1.2.3.4"

	// Primeira requisição passa
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip + ":5678"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Segunda requisição deve ser bloqueada
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip + ":5678"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	retryAfter := w.Header().Get("Retry-After")
	if retryAfter != "60" {
		t.Errorf("expected Retry-After header '60', got '%s'", retryAfter)
	}
}

// TestRateLimit_ExtractsRealIP verifica se o header X-Real-IP é extraído corretamente.
func TestRateLimit_ExtractsRealIP(t *testing.T) {
	limiter := NewRateLimiter(1) // 1 requisição por minuto
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Primeira requisição com X-Real-IP
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Real-IP", "10.0.0.1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("first request: expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Segunda requisição com mesmo X-Real-IP (deve ser bloqueada)
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Real-IP", "10.0.0.1")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request with same X-Real-IP: expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Terceira requisição com IP diferente em X-Real-IP (deve passar)
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Real-IP", "10.0.0.2")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("third request with different X-Real-IP: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimit_ResponseJSON verifica se a resposta bloqueada é um JSON válido com campo "error".
func TestRateLimit_ResponseJSON(t *testing.T) {
	limiter := NewRateLimiter(1) // 1 requisição por minuto
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "1.2.3.4"

	// Primeira requisição passa
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip + ":5678"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Segunda requisição deve ser bloqueada
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip + ":5678"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Verifica se o Content-Type é JSON
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Parse e valida o JSON
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("failed to parse response JSON: %v", err)
	}

	if _, ok := resp["error"]; !ok {
		t.Errorf("expected 'error' field in JSON response, got: %v", resp)
	}
}

// TestRateLimit_XForwardedFor verifica se o header X-Forwarded-For é extraído corretamente.
func TestRateLimit_XForwardedFor(t *testing.T) {
	limiter := NewRateLimiter(1) // 1 requisição por minuto
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Primeira requisição com X-Forwarded-For
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2, 10.0.0.3")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("first request: expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Segunda requisição com mesmo X-Forwarded-For (primeiro IP deve ser usado)
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.4, 10.0.0.5")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected status %d (rate limited), got %d", http.StatusTooManyRequests, w.Code)
	}
}

// TestRateLimit_RemoteAddrWithoutPort verifica se RemoteAddr sem porta é tratado corretamente.
func TestRateLimit_RemoteAddrWithoutPort(t *testing.T) {
	limiter := NewRateLimiter(1)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Primeira requisição
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("first request: expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Segunda requisição do mesmo IP (sem porta)
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}
