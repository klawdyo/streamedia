package admin

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// mustExec insere um vídeo "ready" já vinculado a um projeto — usado para
// testar o escopo de admin por projeto (T33, issue #6).
func mustExec(t *testing.T, database *sql.DB, videoID string, projectID *int64) {
	t.Helper()
	_, err := database.Exec(
		"INSERT INTO videos (video_id, status, project_id, created_at, updated_at) VALUES (?, 'ready', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		videoID, projectID,
	)
	if err != nil {
		t.Fatalf("erro ao inserir vídeo %s: %v", videoID, err)
	}
}

// TestAdminAuth_ScopedToProject verifica a decisão de modelo admin adotada
// (opção (a) do spec da T33): o ADMIN_TOKEN global continua enxergando
// tudo (super-admin, sem escopo), enquanto a chave mestra de um projeto
// autentica em /admin/* mas só enxerga/opera os vídeos daquele projeto —
// nunca os de outro.
func TestAdminAuth_ScopedToProject(t *testing.T) {
	database, cfg := setupAdminTest(t)
	handler := NewAdminHandler(cfg, database, &mockQueue{})
	wrapped := AdminAuth(cfg.AdminToken, database)(http.HandlerFunc(handler.HandleVideos))

	projectA, masterKeyA, err := models.CreateProject(database, "Projeto A")
	if err != nil {
		t.Fatalf("CreateProject A: %v", err)
	}
	projectB, masterKeyB, err := models.CreateProject(database, "Projeto B")
	if err != nil {
		t.Fatalf("CreateProject B: %v", err)
	}

	mustExec(t, database, "vid-a-1", &projectA.ID)
	mustExec(t, database, "vid-a-2", &projectA.ID)
	mustExec(t, database, "vid-b-1", &projectB.ID)

	// A chave mestra do Projeto A só enxerga os vídeos do Projeto A.
	respA := doVideosRequest(t, wrapped, masterKeyA)
	if respA.Total != 2 {
		t.Fatalf("Projeto A: esperava 2 vídeos, obteve %d", respA.Total)
	}
	for _, v := range respA.Videos {
		if v.ProjectID == nil || *v.ProjectID != projectA.ID {
			t.Errorf("Projeto A: vídeo %q não deveria aparecer (project_id=%v)", v.VideoID, v.ProjectID)
		}
	}

	// A chave mestra do Projeto B só enxerga os vídeos do Projeto B —
	// nunca os do Projeto A.
	respB := doVideosRequest(t, wrapped, masterKeyB)
	if respB.Total != 1 {
		t.Fatalf("Projeto B: esperava 1 vídeo, obteve %d", respB.Total)
	}
	for _, v := range respB.Videos {
		if v.ProjectID == nil || *v.ProjectID != projectB.ID {
			t.Errorf("Projeto B: vídeo %q não deveria aparecer (project_id=%v)", v.VideoID, v.ProjectID)
		}
	}

	// O ADMIN_TOKEN global continua sendo super-admin: enxerga todos.
	respGlobal := doVideosRequest(t, wrapped, cfg.AdminToken)
	if respGlobal.Total != 3 {
		t.Fatalf("super-admin: esperava 3 vídeos (sem escopo), obteve %d", respGlobal.Total)
	}
}

// doVideosRequest executa GET /admin/videos autenticado com o bearer token
// informado (token global ou chave mestra de projeto) e devolve a resposta decodificada.
func doVideosRequest(t *testing.T, wrapped http.Handler, bearerToken string) videosResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	body, _ := io.ReadAll(rec.Body)
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("erro ao fazer unmarshal da resposta: %v", err)
	}
	dataJSON, _ := json.Marshal(env.Data)
	var resp videosResponse
	json.Unmarshal(dataJSON, &resp)
	return resp
}
