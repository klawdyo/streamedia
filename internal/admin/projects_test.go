package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// newProjectsTestRouter monta um roteador chi mínimo com as rotas de
// gerenciamento de projetos (T35) — necessário porque os handlers usam
// chi.URLParam para extrair {slug} do path, o que exige um RouteContext
// real (não fornecido por httptest.NewRequest sozinho). Reproduz o mesmo
// agrupamento usado em internal/server/server.go: rotas de gerenciamento
// (criar/listar/consultar) atrás do AdminAuth, e a emissão de token de
// upload fora dele (autenticação própria via X-Project-Key).
func newProjectsTestRouter(t *testing.T, handler *AdminHandler, adminToken string, database *sql.DB) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(AdminAuth(adminToken, database))
		r.Post("/admin/projects", handler.HandleCreateProject)
		r.Get("/admin/projects", handler.HandleListProjects)
		r.Get("/admin/projects/{slug}", handler.HandleGetProject)
	})
	r.Post("/admin/projects/{slug}/upload-tokens", handler.HandleIssueUploadToken)
	return r
}

// --- Testes de criação de projeto ---

// TestCreateProject_ReturnsSlugAndMasterKey verifica que POST /admin/projects
// (autenticado como super-admin) cria o projeto e devolve slug, root_dir e a
// chave mestra em texto puro — única vez em que ela é exposta.
func TestCreateProject_ReturnsSlugAndMasterKey(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})
	router := newProjectsTestRouter(t, handler, cfg.AdminToken, database)

	body := bytes.NewBufferString(`{"name": "Acme Studios"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/projects", body)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperava 201, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp createProjectResponse
	json.Unmarshal(dataJSON, &resp)

	if resp.Slug != "acme-studios" {
		t.Errorf("esperava slug 'acme-studios', obteve %q", resp.Slug)
	}
	if resp.RootDir != resp.Slug {
		t.Errorf("esperava root_dir igual ao slug, obteve root_dir=%q slug=%q", resp.RootDir, resp.Slug)
	}
	if resp.MasterKey == "" {
		t.Error("esperava master_key em texto puro na resposta de criação")
	}

	// A chave devolvida deve corresponder ao hash persistido — é a única
	// vez em que ela existe em texto puro fora da memória do cliente.
	project, err := models.GetProjectByMasterKeyHash(database, models.HashMasterKey(resp.MasterKey))
	if err != nil {
		t.Fatalf("a master_key devolvida não corresponde a nenhum projeto: %v", err)
	}
	if project.ID != resp.ID {
		t.Errorf("esperava que a chave resolvesse para o projeto %d, resolveu para %d", resp.ID, project.ID)
	}
}

// TestCreateProject_RequiresAdminAuth verifica que POST /admin/projects
// rejeita tanto requisições sem autenticação (401) quanto requisições
// autenticadas com a chave mestra de um projeto (403) — só o ADMIN_TOKEN
// global pode criar novos projetos (ver requireSuperAdmin).
func TestCreateProject_RequiresAdminAuth(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})
	router := newProjectsTestRouter(t, handler, cfg.AdminToken, database)

	// Sem autenticação.
	reqNoAuth := httptest.NewRequest(http.MethodPost, "/admin/projects", bytes.NewBufferString(`{"name": "X"}`))
	recNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recNoAuth, reqNoAuth)
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("sem autenticação: esperava 401, obteve %d", recNoAuth.Code)
	}

	// Autenticado com a chave mestra de um projeto já existente — não deve
	// poder criar outros projetos (operação de super-admin).
	_, masterKey, err := models.CreateProject(database, "Projeto Existente")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	reqScoped := httptest.NewRequest(http.MethodPost, "/admin/projects", bytes.NewBufferString(`{"name": "Outro"}`))
	reqScoped.Header.Set("Authorization", "Bearer "+masterKey)
	recScoped := httptest.NewRecorder()
	router.ServeHTTP(recScoped, reqScoped)
	if recScoped.Code != http.StatusForbidden {
		t.Errorf("autenticado com chave de projeto: esperava 403, obteve %d: %s", recScoped.Code, recScoped.Body.String())
	}
}

// --- Testes de listagem e consulta ---

// TestListProjects_OmitsMasterKeyHash verifica que GET /admin/projects
// devolve os projetos cadastrados sem nunca expor master_key ou
// master_key_hash — apenas campos públicos.
func TestListProjects_OmitsMasterKeyHash(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})
	router := newProjectsTestRouter(t, handler, cfg.AdminToken, database)

	if _, _, err := models.CreateProject(database, "Projeto Um"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, _, err := models.CreateProject(database, "Projeto Dois"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/projects", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}

	bodyStr := rec.Body.String()
	if bytes.Contains([]byte(bodyStr), []byte("master_key")) {
		t.Errorf("a resposta de listagem não deveria conter 'master_key' (hash ou texto puro): %s", bodyStr)
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp listProjectsResponse
	json.Unmarshal(dataJSON, &resp)
	if resp.Total != 2 {
		t.Errorf("esperava 2 projetos, obteve %d", resp.Total)
	}
}

// TestGetProject_NotFound verifica que GET /admin/projects/{slug} devolve
// 404 quando o slug não corresponde a nenhum projeto cadastrado.
func TestGetProject_NotFound(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})
	router := newProjectsTestRouter(t, handler, cfg.AdminToken, database)

	req := httptest.NewRequest(http.MethodGet, "/admin/projects/nao-existe", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("esperava 404, obteve %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Testes de emissão de token de upload ---

// TestIssueUploadToken_RequiresProjectMasterKey verifica que
// POST /admin/projects/{slug}/upload-tokens exige a chave mestra do PRÓPRIO
// projeto via X-Project-Key — nem ausência de chave, nem a chave de outro
// projeto, nem o ADMIN_TOKEN global substituem essa autenticação (ver a
// justificativa de modelo em HandleIssueUploadToken: é o mesmo princípio de
// /upload/init, "apresente a chave mestra para obter credenciais de upload").
func TestIssueUploadToken_RequiresProjectMasterKey(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})
	router := newProjectsTestRouter(t, handler, cfg.AdminToken, database)

	projectA, masterKeyA, err := models.CreateProject(database, "Projeto A")
	if err != nil {
		t.Fatalf("CreateProject A: %v", err)
	}
	_, masterKeyB, err := models.CreateProject(database, "Projeto B")
	if err != nil {
		t.Fatalf("CreateProject B: %v", err)
	}

	path := "/admin/projects/" + projectA.Slug + "/upload-tokens"

	// Sem X-Project-Key.
	reqNoKey := httptest.NewRequest(http.MethodPost, path, nil)
	recNoKey := httptest.NewRecorder()
	router.ServeHTTP(recNoKey, reqNoKey)
	if recNoKey.Code != http.StatusUnauthorized {
		t.Errorf("sem X-Project-Key: esperava 401, obteve %d", recNoKey.Code)
	}

	// Com a chave mestra de OUTRO projeto.
	reqWrongKey := httptest.NewRequest(http.MethodPost, path, nil)
	reqWrongKey.Header.Set("X-Project-Key", masterKeyB)
	recWrongKey := httptest.NewRecorder()
	router.ServeHTTP(recWrongKey, reqWrongKey)
	if recWrongKey.Code != http.StatusForbidden {
		t.Errorf("com chave de outro projeto: esperava 403, obteve %d: %s", recWrongKey.Code, recWrongKey.Body.String())
	}

	// Com o ADMIN_TOKEN global no lugar da chave de projeto — não é aceito
	// aqui (esta rota não usa AdminAuth, e o token global não é uma chave de
	// projeto válida).
	reqAdminToken := httptest.NewRequest(http.MethodPost, path, nil)
	reqAdminToken.Header.Set("X-Project-Key", cfg.AdminToken)
	recAdminToken := httptest.NewRecorder()
	router.ServeHTTP(recAdminToken, reqAdminToken)
	if recAdminToken.Code != http.StatusUnauthorized {
		t.Errorf("com ADMIN_TOKEN como X-Project-Key: esperava 401, obteve %d", recAdminToken.Code)
	}

	// Com a própria chave mestra do projeto: sucesso.
	reqOK := httptest.NewRequest(http.MethodPost, path, nil)
	reqOK.Header.Set("X-Project-Key", masterKeyA)
	recOK := httptest.NewRecorder()
	router.ServeHTTP(recOK, reqOK)
	if recOK.Code != http.StatusCreated {
		t.Fatalf("com a própria chave mestra: esperava 201, obteve %d: %s", recOK.Code, recOK.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(recOK.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp issueUploadTokenResponse
	json.Unmarshal(dataJSON, &resp)
	if resp.VideoID == "" || resp.Token == "" {
		t.Errorf("esperava video_id e token preenchidos na resposta: %+v", resp)
	}

	// O vídeo deve ter sido registrado e associado ao projeto correto.
	video, err := models.GetVideo(database, resp.VideoID)
	if err != nil {
		t.Fatalf("esperava vídeo %q registrado: %v", resp.VideoID, err)
	}
	if video.ProjectID == nil || *video.ProjectID != projectA.ID {
		t.Errorf("esperava vídeo associado ao Projeto A (id=%d), obteve project_id=%v", projectA.ID, video.ProjectID)
	}
}

// TestIssueUploadToken_ShortTTL verifica que o token emitido por
// POST /admin/projects/{slug}/upload-tokens expira dentro da janela curta
// configurada para uploads escopados (UploadTokenScopedTTL, ~15-20min,
// T33) — não o TTL longo do fluxo legado.
func TestIssueUploadToken_ShortTTL(t *testing.T) {
	database, cfg := setupAdminTest(t)
	cfg.UploadTokenScopedTTL = 18 * time.Minute
	handler := NewAdminHandler(cfg, database, &mockQueue{})
	router := newProjectsTestRouter(t, handler, cfg.AdminToken, database)

	project, masterKey, err := models.CreateProject(database, "Short TTL Co")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/projects/"+project.Slug+"/upload-tokens", nil)
	req.Header.Set("X-Project-Key", masterKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperava 201, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp issueUploadTokenResponse
	json.Unmarshal(dataJSON, &resp)

	expiresAt, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at não está em RFC3339: %v", err)
	}

	ttl := time.Until(expiresAt)
	// Janela de tolerância: entre 15 e 20 minutos (conforme a recomendação
	// da T33 para tokens de upload escopados a um único arquivo).
	if ttl < 15*time.Minute || ttl > 20*time.Minute {
		t.Errorf("esperava TTL entre 15 e 20 minutos, obteve %v (expires_at=%s)", ttl, resp.ExpiresAt)
	}

	// Confere também o registro persistido no banco.
	persisted, err := models.GetUploadTokenByVideoID(database, resp.VideoID)
	if err != nil {
		t.Fatalf("erro ao buscar token persistido: %v", err)
	}
	persistedTTL := persisted.ExpiresAt.Sub(time.Now())
	if persistedTTL < 15*time.Minute || persistedTTL > 20*time.Minute {
		t.Errorf("token persistido: esperava TTL entre 15 e 20 minutos, obteve %v", persistedTTL)
	}
}
