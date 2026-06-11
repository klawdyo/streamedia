package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
)

// sessionTestConfig retorna uma configuração mínima para os testes de
// sessão: ROOT_TOKEN fixo, TTL curto e cookie não-Secure (testes rodam por
// HTTP simulado, não HTTPS).
func sessionTestConfig() *config.Config {
	return &config.Config{
		RootToken:           "test-root-token",
		SessionTTL:          time.Hour,
		SessionCookieSecure: false,
	}
}

// TestHandleSessionLogin_ValidBearer verifica que um Bearer válido recebe um
// cookie streamedia_session com os atributos de segurança esperados.
func TestHandleSessionLogin_ValidBearer(t *testing.T) {
	cfg := sessionTestConfig()

	req := httptest.NewRequest(http.MethodPost, "/admin/session", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	w := httptest.NewRecorder()

	HandleSessionLogin(cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status esperado 200, obtido %d", w.Code)
	}

	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("esperado 1 cookie, obtido %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName {
		t.Errorf("nome do cookie esperado %q, obtido %q", SessionCookieName, c.Name)
	}
	if !c.HttpOnly {
		t.Error("cookie de sessão deveria ser HttpOnly")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite esperado Strict, obtido %v", c.SameSite)
	}
	if c.Secure != cfg.SessionCookieSecure {
		t.Errorf("Secure esperado %v, obtido %v", cfg.SessionCookieSecure, c.Secure)
	}
	if c.Path != "/" {
		t.Errorf("Path esperado \"/\", obtido %q", c.Path)
	}
	if !auth.ValidateSessionToken(cfg.RootToken, c.Value) {
		t.Error("o valor do cookie deveria ser um token de sessão válido")
	}
}

// TestHandleSessionLogin_InvalidBearer verifica que sem Bearer válido a
// resposta é 401 e nenhum cookie é definido.
func TestHandleSessionLogin_InvalidBearer(t *testing.T) {
	cfg := sessionTestConfig()

	req := httptest.NewRequest(http.MethodPost, "/admin/session", nil)
	req.Header.Set("Authorization", "Bearer token-errado")
	w := httptest.NewRecorder()

	HandleSessionLogin(cfg)(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status esperado 401, obtido %d", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Error("não deveria definir cookie quando o Bearer é inválido")
	}
}

// TestHandleSessionLogout_ClearsCookie verifica que o logout responde 200 e
// envia um Set-Cookie que apaga o cookie de sessão (MaxAge negativo).
func TestHandleSessionLogout_ClearsCookie(t *testing.T) {
	cfg := sessionTestConfig()

	req := httptest.NewRequest(http.MethodDelete, "/admin/session", nil)
	w := httptest.NewRecorder()

	HandleSessionLogout(cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status esperado 200, obtido %d", w.Code)
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("esperado 1 cookie, obtido %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName {
		t.Errorf("nome do cookie esperado %q, obtido %q", SessionCookieName, c.Name)
	}
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge deveria ser negativo (apaga o cookie), obtido %d", c.MaxAge)
	}
}

// TestRootAuth_AcceptsSessionCookie verifica que RootAuth autentica uma
// requisição GET autenticada apenas pelo cookie de sessão (sem Bearer).
func TestRootAuth_AcceptsSessionCookie(t *testing.T) {
	cfg := sessionTestConfig()

	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	value, _ := auth.IssueSessionToken(cfg.RootToken, cfg.SessionTTL)
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: value})
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status esperado 200, obtido %d", w.Code)
	}
}

// TestRootAuth_RejectsExpiredSessionCookie verifica que um cookie de sessão
// expirado é rejeitado com 401.
func TestRootAuth_RejectsExpiredSessionCookie(t *testing.T) {
	cfg := sessionTestConfig()

	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	value, _ := auth.IssueSessionToken(cfg.RootToken, -time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: value})
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status esperado 401, obtido %d", w.Code)
	}
}

// TestRootAuth_CSRF verifica que requisições com método não seguro (DELETE)
// autenticadas via cookie de sessão exigem o header X-Streamedia-Csrf: sem o
// header retorna 403, com o header passa pelo middleware.
func TestRootAuth_CSRF(t *testing.T) {
	cfg := sessionTestConfig()

	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	value, _ := auth.IssueSessionToken(cfg.RootToken, cfg.SessionTTL)

	// Sem o header CSRF: 403.
	req := httptest.NewRequest(http.MethodDelete, "/admin/videos/abc", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: value})
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("sem header CSRF: status esperado 403, obtido %d", w.Code)
	}

	// Com o header CSRF: passa para o handler (200).
	req2 := httptest.NewRequest(http.MethodDelete, "/admin/videos/abc", nil)
	req2.AddCookie(&http.Cookie{Name: SessionCookieName, Value: value})
	req2.Header.Set(CSRFHeaderName, "1")
	w2 := httptest.NewRecorder()
	wrapped.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("com header CSRF: status esperado 200, obtido %d", w2.Code)
	}
}

// TestRootAuth_BearerNotSubjectToCSRF verifica que requisições autenticadas
// via Authorization: Bearer não precisam do header X-Streamedia-Csrf, mesmo
// em métodos não seguros.
func TestRootAuth_BearerNotSubjectToCSRF(t *testing.T) {
	cfg := sessionTestConfig()

	wrapped := RootAuth(cfg.RootToken)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/admin/videos/abc", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.RootToken)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status esperado 200, obtido %d", w.Code)
	}
}
