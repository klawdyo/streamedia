// Rotas de gerenciamento de projetos (T35, issue #6): expõem via HTTP o
// CRUD básico de projetos (T32) e a emissão de tokens de upload escopados
// (T33), fechando o ciclo "criar projeto → consultar → emitir credenciais
// de upload para um vídeo".
package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/httputil"
	"github.com/klawdyo/streamedia/internal/models"
)

// requireSuperAdmin garante que a requisição foi autenticada pelo
// ADMIN_TOKEN global (sem escopo de projeto). As rotas de gerenciamento de
// projetos (criar, listar, consultar) são operações sensíveis — quem pode
// criar um projeto e gerar sua chave mestra precisa estar acima de qualquer
// projeto individual, então a autenticação por chave mestra de projeto
// (aceita pelo AdminAuth para as demais rotas /admin/*) é rejeitada aqui.
// Retorna false (e já escreve a resposta de erro) se a checagem falhar.
func requireSuperAdmin(w http.ResponseWriter, r *http.Request) bool {
	if scope := ProjectScopeFromContext(r.Context()); scope != nil {
		apiresponse.Error(w, http.StatusForbidden, "Esta operação exige autenticação de super-admin (ADMIN_TOKEN global).")
		return false
	}
	return true
}

// projectResponse é a representação pública de um projeto — nunca inclui
// MasterKeyHash (e muito menos a chave em texto puro, exceto na criação).
type projectResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	RootDir   string `json:"root_dir"`
	CreatedAt string `json:"created_at"`
}

// toProjectResponse converte um *models.Project para sua representação
// pública, omitindo deliberadamente MasterKeyHash.
func toProjectResponse(p *models.Project) projectResponse {
	return projectResponse{
		ID:        p.ID,
		Name:      p.Name,
		Slug:      p.Slug,
		RootDir:   p.RootDir,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// createProjectRequest é o corpo esperado em POST /admin/projects.
type createProjectRequest struct {
	Name string `json:"name"`
}

// createProjectResponse inclui a chave mestra em texto puro — ela só existe
// neste momento; o chamador deve guardá-la, pois o servidor jamais a
// devolve novamente (apenas o hash é persistido).
type createProjectResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	RootDir   string `json:"root_dir"`
	MasterKey string `json:"master_key"`
}

// HandleCreateProject cria um novo projeto a partir de {"name": "..."}.
// Operação de super-admin: cria o namespace e sua chave mestra, então exige
// o ADMIN_TOKEN global (ver requireSuperAdmin). Devolve a chave mestra em
// texto puro — única vez em que ela é exposta.
func (h *AdminHandler) HandleCreateProject(w http.ResponseWriter, r *http.Request) {
	if !requireSuperAdmin(w, r) {
		return
	}

	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		apiresponse.Error(w, http.StatusBadRequest, "O campo 'name' é obrigatório.")
		return
	}

	project, masterKey, err := models.CreateProject(h.db, req.Name)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao criar o projeto.")
		return
	}

	apiresponse.Success(w, http.StatusCreated, createProjectResponse{
		ID:        project.ID,
		Name:      project.Name,
		Slug:      project.Slug,
		RootDir:   project.RootDir,
		MasterKey: masterKey,
	})
}

// listProjectsResponse é a estrutura de resposta de GET /admin/projects.
type listProjectsResponse struct {
	Projects []projectResponse `json:"projects"`
	Total    int               `json:"total"`
}

// HandleListProjects lista todos os projetos cadastrados, sem expor o hash
// da chave mestra. Operação de super-admin (ver requireSuperAdmin) — uma
// chave de projeto não deveria enxergar o catálogo completo de projetos.
func (h *AdminHandler) HandleListProjects(w http.ResponseWriter, r *http.Request) {
	if !requireSuperAdmin(w, r) {
		return
	}

	projects, err := models.ListProjects(h.db)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao listar projetos.")
		return
	}

	resp := listProjectsResponse{
		Projects: make([]projectResponse, 0, len(projects)),
		Total:    len(projects),
	}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, toProjectResponse(p))
	}

	apiresponse.Success(w, http.StatusOK, resp)
}

// HandleGetProject devolve o detalhe de um projeto pelo seu slug, sem expor
// o hash da chave mestra. Operação de super-admin (ver requireSuperAdmin).
func (h *AdminHandler) HandleGetProject(w http.ResponseWriter, r *http.Request) {
	if !requireSuperAdmin(w, r) {
		return
	}

	slug := chi.URLParam(r, "slug")
	project, err := models.GetProjectBySlug(h.db, slug)
	if errors.Is(err, sql.ErrNoRows) {
		apiresponse.Error(w, http.StatusNotFound, "Projeto não encontrado.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao consultar o projeto.")
		return
	}

	apiresponse.Success(w, http.StatusOK, toProjectResponse(project))
}

// issueUploadTokenRequest é o corpo opcional de
// POST /admin/projects/{slug}/upload-tokens — permite informar o tamanho
// declarado do arquivo (mesmo campo aceito por POST /upload/init).
type issueUploadTokenRequest struct {
	DeclaredSizeBytes int64 `json:"declared_size_bytes"`
}

// issueUploadTokenResponse espelha a resposta de POST /upload/init — é,
// na prática, o mesmo fluxo, só que com video_id gerado pelo servidor.
type issueUploadTokenResponse struct {
	VideoID   string `json:"video_id"`
	UploadURL string `json:"upload_url"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// HandleIssueUploadToken troca a chave mestra de um projeto por um token de
// upload de curta duração para um video_id recém-gerado pelo servidor —
// issue #6, T35.
//
// Decisão de autenticação (diferente das demais rotas deste arquivo): esta
// rota NÃO exige o ADMIN_TOKEN global. Ela representa a operação inversa de
// "apresentar a chave mestra para obter credenciais de upload" — exatamente
// o que POST /upload/init já faz no fluxo escopado a projeto (T33), só que
// aqui o video_id é gerado pelo servidor (UUID v4) em vez de informado pelo
// cliente. Por isso a autenticação natural é a própria chave mestra do
// projeto, apresentada via X-Project-Key (mesmo header e mesmo princípio de
// "nunca reter a chave em texto puro": o servidor calcula o hash e resolve
// o projeto). O {slug} no path é validado contra o projeto resolvido pela
// chave — proteção extra contra o uso da chave de um projeto para emitir um
// token "rotulado" como de outro.
//
// Reaproveita a mesma assinatura HMAC (auth.GenerateUploadToken com a chave
// mestra como segredo) e o mesmo TTL (UploadTokenTTL) do fluxo de
// /upload/init — consistência total com T33 e issue #10 (T50).
func (h *AdminHandler) HandleIssueUploadToken(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	projectKey := r.Header.Get("X-Project-Key")
	if projectKey == "" {
		apiresponse.Error(w, http.StatusUnauthorized, "Header X-Project-Key é obrigatório.")
		return
	}

	project, err := models.GetProjectByMasterKeyHash(h.db, models.HashMasterKey(projectKey))
	if errors.Is(err, sql.ErrNoRows) {
		apiresponse.Error(w, http.StatusUnauthorized, "Chave de projeto inválida.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao validar a chave de projeto.")
		return
	}
	if project.Slug != slug {
		apiresponse.Error(w, http.StatusForbidden, "A chave informada não pertence a este projeto.")
		return
	}

	// Corpo é opcional — declared_size_bytes pode ser omitido (0 = desconhecido).
	var req issueUploadTokenRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
			return
		}
	}
	if req.DeclaredSizeBytes < 0 {
		apiresponse.Error(w, http.StatusBadRequest, "declared_size_bytes não pode ser negativo.")
		return
	}

	// Gera o video_id via função centralizada — o sistema sempre privilegia
	// UUID v7 ao gerar ids (ordenável por tempo, melhora localidade no SQLite).
	videoID, err := models.NewVideoID()
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao gerar video_id.")
		return
	}
	if err := models.InsertVideoForProject(h.db, videoID, req.DeclaredSizeBytes, &project.ID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o vídeo.")
		return
	}

	token := auth.GenerateUploadToken(projectKey, videoID)
	expiresAt := time.Now().Add(h.cfg.UploadTokenTTL)
	if err := models.InsertUploadTokenForProject(h.db, token, videoID, expiresAt, &project.ID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Falha ao registrar o token de upload.")
		return
	}

	// Constrói a URL de upload usando a função centralizada (httputil).
	uploadURL := httputil.PublicUploadURL(r, videoID)

	apiresponse.Success(w, http.StatusCreated, issueUploadTokenResponse{
		VideoID:   videoID,
		UploadURL: uploadURL,
		Token:     token,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	})
}
