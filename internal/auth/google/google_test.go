package google

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// --- Helpers de teste ---

// googleTestConfig retorna uma configuração mínima para os testes de OAuth.
func googleTestConfig() *config.Config {
	return &config.Config{
		RootToken:           "test-root-token",
		SessionTTL:          time.Hour,
		SessionCookieSecure: false,
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
	}
}

// setupGoogleTest cria um banco em memória com migrations e retorna a conexão
// e o handler de autenticação Google.
func setupGoogleTest(t *testing.T) (*sql.DB, *GoogleHandler, *config.Config) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := googleTestConfig()
	handler := NewGoogleHandler(cfg, database)

	return database, handler, cfg
}

// fakeGoogleServer cria um servidor HTTP falso que simula os endpoints OAuth
// do Google (token e userinfo). Retorna a URL base do servidor e uma função
// de limpeza.
func fakeGoogleServer(t *testing.T, email, name, picture string) (string, func()) {
	t.Helper()

	mux := http.NewServeMux()

	// Endpoint de token: sempre retorna um access_token fixo.
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})

	// Endpoint userinfo: retorna os dados do usuário fake.
	mux.HandleFunc("/oauth2/v2/userinfo", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer fake-access-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"email":   email,
			"name":    name,
			"picture": picture,
		})
	})

	srv := httptest.NewServer(mux)
	return srv.URL, func() { srv.Close() }
}

// handlerWithFakeGoogle cria um GoogleHandler que aponta para um servidor
// Google falso, para testes de callback.
func handlerWithFakeGoogle(t *testing.T, email, name, picture string) (*sql.DB, *GoogleHandler, *config.Config, func()) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}

	cfg := googleTestConfig()
	handler := NewGoogleHandler(cfg, database)

	// Injeta servidor Google falso.
	googleURL, cleanup := fakeGoogleServer(t, email, name, picture)
	handler.tokenURL = googleURL + "/token"
	handler.userInfoURL = googleURL + "/oauth2/v2/userinfo"

	cleanupAll := func() {
		cleanup()
		_ = database.Close()
	}
	t.Cleanup(cleanupAll)

	return database, handler, cfg, cleanupAll
}

// --- Testes ---

// TestHandleLogin_URLConstruction verifica que HandleLogin redireciona para
// a URL correta do Google com todos os parâmetros OAuth necessários e define
// o cookie de state.
func TestHandleLogin_URLConstruction(t *testing.T) {
	_, handler, _ := setupGoogleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google", nil)
	w := httptest.NewRecorder()

	handler.HandleLogin(w, req)

	// Verifica status 302 (Found).
	if w.Code != http.StatusFound {
		t.Fatalf("status esperado 302, obtido %d", w.Code)
	}

	// Verifica a URL de redirecionamento.
	location := w.Header().Get("Location")
	if location == "" {
		t.Fatal("header Location está vazio")
	}

	if !strings.HasPrefix(location, "https://accounts.google.com/o/oauth2/v2/auth") {
		t.Errorf("URL de redirecionamento deveria começar com accounts.google.com, obtido %q", location)
	}

	// Verifica parâmetros obrigatórios na URL.
	requiredParams := []string{
		"client_id=test-client-id",
		"redirect_uri=http%3A%2F%2Fexample.com%2Fapi%2Fauth%2Fgoogle%2Fcallback",
		"response_type=code",
		"scope=openid+profile+email",
		"state=",
	}
	for _, param := range requiredParams {
		if !strings.Contains(location, param) {
			t.Errorf("URL deveria conter %q, obtido %q", param, location)
		}
	}

	// Verifica que o state é um hex de 64 caracteres (32 bytes).
	stateIdx := strings.Index(location, "state=")
	if stateIdx == -1 {
		t.Fatal("parâmetro state não encontrado na URL")
	}
	stateVal := location[stateIdx+len("state="):]
	if ampersandIdx := strings.Index(stateVal, "&"); ampersandIdx != -1 {
		stateVal = stateVal[:ampersandIdx]
	}
	if len(stateVal) != 64 {
		t.Errorf("state deveria ter 64 caracteres hex, obtido %d: %q", len(stateVal), stateVal)
	}

	// Verifica que o cookie de state foi definido.
	cookies := w.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == stateCookieName {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("cookie de state não foi definido")
	}
	if stateCookie.Value == "" {
		t.Error("cookie de state tem valor vazio")
	}
	if stateCookie.Value != stateVal {
		t.Errorf("cookie state %q diferente do state na URL %q", stateCookie.Value, stateVal)
	}
	if !stateCookie.HttpOnly {
		t.Error("cookie de state deveria ser HttpOnly")
	}
	if stateCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite esperado Lax, obtido %v", stateCookie.SameSite)
	}
}

// TestHandleCallback_InvalidState verifica que um callback com state
// inválido (diferente do cookie) é rejeitado com erro 400.
func TestHandleCallback_InvalidState(t *testing.T) {
	_, handler, _ := setupGoogleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=test-code&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "correct-state",
	})
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status esperado 400 para state inválido, obtido %d", w.Code)
	}
}

// TestHandleCallback_Bootstrap verifica que o primeiro login (tabela users
// vazia) cria o usuário com role "dev" e emite cookie de sessão.
func TestHandleCallback_Bootstrap(t *testing.T) {
	database, handler, cfg, _ := handlerWithFakeGoogle(t, "dev@streamedia.com", "Dev User", "https://pic.example.com/dev.jpg")

	// Confirma que o banco está vazio antes do bootstrap.
	count, err := models.CountUsers(database)
	if err != nil {
		t.Fatalf("erro ao contar usuários: %v", err)
	}
	if count != 0 {
		t.Fatalf("banco deveria estar vazio antes do bootstrap, tem %d usuários", count)
	}

	// Simula o callback do Google: state nos cookies e na query batem.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=test-auth-code&state=test-state-value", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "test-state-value",
	})
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	// Verifica redirecionamento para /app.
	if w.Code != http.StatusFound {
		t.Fatalf("status esperado 302, obtido %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/app" {
		t.Errorf("Location esperado /app, obtido %q", loc)
	}

	// Verifica que o usuário foi criado.
	user, err := models.GetUserByEmail(database, "dev@streamedia.com")
	if err != nil {
		t.Fatalf("erro ao buscar usuário bootstrap: %v", err)
	}
	if user.Email != "dev@streamedia.com" {
		t.Errorf("email esperado %q, obtido %q", "dev@streamedia.com", user.Email)
	}
	if user.Name != "Dev User" {
		t.Errorf("nome esperado %q, obtido %q", "Dev User", user.Name)
	}
	if user.Picture != "https://pic.example.com/dev.jpg" {
		t.Errorf("picture esperada %q, obtida %q", "https://pic.example.com/dev.jpg", user.Picture)
	}

	// Verifica que a role "dev" foi concedida.
	roles, err := models.GetUserRoles(database, user.ID)
	if err != nil {
		t.Fatalf("erro ao buscar roles: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("esperado 1 role, obtido %d", len(roles))
	}
	if roles[0].Role != models.RoleDev {
		t.Errorf("role esperada %q, obtida %q", models.RoleDev, roles[0].Role)
	}
	if roles[0].LevelNum != 1 {
		t.Errorf("level_num esperado 1, obtido %d", roles[0].LevelNum)
	}

	// Verifica que o cookie de sessão foi emitido com o novo formato.
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("cookie de sessão não foi definido")
	}

	// Valida o token de sessão no novo formato (com user_id).
	expUnix, uid, csv, ok := auth.ValidateSessionTokenWithUser(cfg.RootToken, sessionCookie.Value)
	if !ok {
		t.Fatal("token de sessão no novo formato deveria ser válido")
	}
	if expUnix <= time.Now().Unix() {
		t.Error("token de sessão expirado")
	}
	if uid != user.ID {
		t.Errorf("user_id esperado %d, obtido %d", user.ID, uid)
	}
	if csv != models.RoleDev {
		t.Errorf("roles esperadas %q, obtidas %q", models.RoleDev, csv)
	}

	// Verifica atributos de segurança do cookie.
	if !sessionCookie.HttpOnly {
		t.Error("cookie de sessão deveria ser HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite esperado Strict, obtido %v", sessionCookie.SameSite)
	}
	if sessionCookie.Path != "/" {
		t.Errorf("Path esperado \"/\", obtido %q", sessionCookie.Path)
	}
}

// TestHandleCallback_EmailNotAuthorized verifica que um email não cadastrado
// (com users já populado) recebe 403.
func TestHandleCallback_EmailNotAuthorized(t *testing.T) {
	database, handler, _, _ := handlerWithFakeGoogle(t, "unknown@streamedia.com", "Unknown User", "")

	// Insere um usuário qualquer para que a tabela não esteja vazia.
	_, err := models.InsertUser(database, "existing@streamedia.com", "Existing User", "")
	if err != nil {
		t.Fatalf("erro ao inserir usuário existente: %v", err)
	}

	// Simula callback para um email não cadastrado.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "test-state",
	})
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status esperado 403 para email não autorizado, obtido %d", w.Code)
	}
}

// TestHandleCallback_ExistingUser verifica que um usuário já cadastrado faz
// login normalmente, com atualização de name/picture e emissão de cookie.
func TestHandleCallback_ExistingUser(t *testing.T) {
	database, handler, cfg, _ := handlerWithFakeGoogle(t, "user@streamedia.com", "Updated Name", "https://pic.example.com/new.jpg")

	// Insere o usuário previamente (simula cadastro anterior).
	userID, err := models.InsertUser(database, "user@streamedia.com", "Old Name", "https://pic.example.com/old.jpg")
	if err != nil {
		t.Fatalf("erro ao inserir usuário: %v", err)
	}
	if err := models.InsertUserRole(database, userID, models.RoleManager, 0); err != nil {
		t.Fatalf("erro ao inserir role: %v", err)
	}

	// Simula callback.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "test-state",
	})
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	// Verifica redirecionamento.
	if w.Code != http.StatusFound {
		t.Fatalf("status esperado 302, obtido %d", w.Code)
	}

	// Verifica que name e picture foram atualizados.
	user, err := models.GetUserByID(database, userID)
	if err != nil {
		t.Fatalf("erro ao buscar usuário: %v", err)
	}
	if user.Name != "Updated Name" {
		t.Errorf("nome esperado %q, obtido %q", "Updated Name", user.Name)
	}
	if user.Picture != "https://pic.example.com/new.jpg" {
		t.Errorf("picture esperada %q, obtida %q", "https://pic.example.com/new.jpg", user.Picture)
	}

	// Verifica que o cookie de sessão tem a role correta.
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("cookie de sessão não foi definido")
	}

	_, _, csv, ok := auth.ValidateSessionTokenWithUser(cfg.RootToken, sessionCookie.Value)
	if !ok {
		t.Fatal("token de sessão deveria ser válido")
	}
	if csv != models.RoleManager {
		t.Errorf("roles esperadas %q, obtidas %q", models.RoleManager, csv)
	}
}

// TestHandleMe_ReturnsUserData verifica que HandleMe retorna os dados do
// usuário autenticado corretamente.
func TestHandleMe_ReturnsUserData(t *testing.T) {
	database, handler, _ := setupGoogleTest(t)

	// Cria um usuário diretamente no banco.
	userID, err := models.InsertUser(database, "user@streamedia.com", "Test User", "https://pic.example.com/user.jpg")
	if err != nil {
		t.Fatalf("erro ao inserir usuário: %v", err)
	}
	if err := models.InsertUserRole(database, userID, models.RoleAdmin, 0); err != nil {
		t.Fatalf("erro ao inserir role: %v", err)
	}

	// Cria requisição com userID no contexto (simulando o que RootAuth faz).
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	ctx := context.WithValue(req.Context(), auth.UserIDContextKey, userID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.HandleMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status esperado 200, obtido %d", w.Code)
	}

	var resp struct {
		Data struct {
			Email          string `json:"email"`
			Name           string `json:"name"`
			Picture        string `json:"picture"`
			Roles          []struct {
				Role     string `json:"role"`
				LevelNum int    `json:"level_num"`
			} `json:"roles"`
			EffectiveLevel int `json:"effective_level"`
		} `json:"data"`
		Error      bool   `json:"error"`
		Message    string `json:"message"`
		StatusCode int    `json:"status_code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("erro ao decodificar resposta JSON: %v", err)
	}

	if resp.Error {
		t.Errorf("não deveria ser erro, obtido message=%q", resp.Message)
	}
	if resp.Data.Email != "user@streamedia.com" {
		t.Errorf("email esperado %q, obtido %q", "user@streamedia.com", resp.Data.Email)
	}
	if resp.Data.Name != "Test User" {
		t.Errorf("nome esperado %q, obtido %q", "Test User", resp.Data.Name)
	}
	if resp.Data.Picture != "https://pic.example.com/user.jpg" {
		t.Errorf("picture esperada %q, obtida %q", "https://pic.example.com/user.jpg", resp.Data.Picture)
	}
	if len(resp.Data.Roles) != 1 {
		t.Fatalf("esperado 1 role, obtido %d", len(resp.Data.Roles))
	}
	if resp.Data.Roles[0].Role != models.RoleAdmin {
		t.Errorf("role esperada %q, obtida %q", models.RoleAdmin, resp.Data.Roles[0].Role)
	}
	if resp.Data.Roles[0].LevelNum != 2 {
		t.Errorf("level_num esperado 2, obtido %d", resp.Data.Roles[0].LevelNum)
	}
	if resp.Data.EffectiveLevel != 2 {
		t.Errorf("effective_level esperado 2, obtido %d", resp.Data.EffectiveLevel)
	}
}

// TestHandleMe_NoAuth verifica que HandleMe retorna 401 quando o contexto
// não contém userID.
func TestHandleMe_NoAuth(t *testing.T) {
	_, handler, _ := setupGoogleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	// Sem userID no contexto.
	w := httptest.NewRecorder()

	handler.HandleMe(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status esperado 401, obtido %d", w.Code)
	}
}

// TestHandleMe_UserNotFound verifica que HandleMe retorna 404 quando o
// userID do contexto não corresponde a nenhum usuário no banco.
func TestHandleMe_UserNotFound(t *testing.T) {
	_, handler, _ := setupGoogleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	ctx := context.WithValue(req.Context(), auth.UserIDContextKey, int64(99999))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.HandleMe(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status esperado 404, obtido %d", w.Code)
	}
}

// TestHandleCallback_MissingStateCookie verifica que callback sem o cookie
// de state é rejeitado.
func TestHandleCallback_MissingStateCookie(t *testing.T) {
	_, handler, _ := setupGoogleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=test&state=whatever", nil)
	// Sem cookie de state.
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status esperado 400, obtido %d", w.Code)
	}
}

// TestHandleCallback_MissingCode verifica que callback sem o parâmetro code
// é rejeitado.
func TestHandleCallback_MissingCode(t *testing.T) {
	_, handler, _ := setupGoogleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state=test-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  stateCookieName,
		Value: "test-state",
	})
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status esperado 400, obtido %d", w.Code)
	}
}
