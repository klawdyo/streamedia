package upload

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// fazRequestInitComProjectKey constrói um POST /upload/init autenticado com
// a chave mestra de um projeto (X-Project-Key), em vez do HMAC global
// legado — fluxo escopado a projeto (T33, issue #6).
func fazRequestInitComProjectKey(masterKey string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/upload/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project-Key", masterKey)
	return req
}

// TestUploadToken_ExpiresInScopedTTL verifica que um upload autenticado com
// a chave mestra de um projeto gera um token com TTL curto (~20min).
func TestUploadToken_ExpiresInScopedTTL(t *testing.T) {
	cfg := configInit(t)
	cfg.UploadTokenTTL = 18 * time.Minute
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	project, masterKey, err := models.CreateProject(database, "Projeto Upload")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	videoID := "550e8400-e29b-41d4-a716-446655440020"
	body := []byte(`{"video_id":"` + videoID + `","declared_size_bytes":2048}`)
	req := fazRequestInitComProjectKey(masterKey, body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var env apiresponse.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("falha ao decodificar resposta: %v", err)
	}
	data, _ := env.Data.(map[string]interface{})
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatal("token ausente na resposta")
	}

	uploadToken, err := models.GetUploadToken(database, token)
	if err != nil {
		t.Fatalf("GetUploadToken: %v", err)
	}

	if uploadToken.ProjectID == nil || *uploadToken.ProjectID != project.ID {
		t.Fatalf("token deveria estar vinculado ao projeto %d, obteve %v", project.ID, uploadToken.ProjectID)
	}
	if uploadToken.VideoID != videoID {
		t.Errorf("token deveria estar vinculado ao vídeo %q, obteve %q", videoID, uploadToken.VideoID)
	}

	gotTTL := uploadToken.ExpiresAt.Sub(time.Now())
	if gotTTL <= 0 || gotTTL > cfg.UploadTokenTTL {
		t.Errorf("TTL do token deveria ser curto (~%s), obteve %s", cfg.UploadTokenTTL, gotTTL)
	}

	// O vídeo também deve carregar o vínculo com o projeto.
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo: %v", err)
	}
	if video.ProjectID == nil || *video.ProjectID != project.ID {
		t.Errorf("vídeo deveria estar vinculado ao projeto %d, obteve %v", project.ID, video.ProjectID)
	}
}

// TestUploadToken_RejectsForOtherProject verifica que a chave mestra de um
// projeto não autentica uploads em nome de outro — cada projeto só pode
// gerar tokens de upload para si mesmo.
func TestUploadToken_RejectsForOtherProject(t *testing.T) {
	cfg := configInit(t)
	database := abreDBInit(t)
	handler := NewInitHandler(cfg, database)

	if _, _, err := models.CreateProject(database, "Projeto A"); err != nil {
		t.Fatalf("CreateProject A: %v", err)
	}
	_, masterKeyB, err := models.CreateProject(database, "Projeto B")
	if err != nil {
		t.Fatalf("CreateProject B: %v", err)
	}

	// Chave de um projeto inexistente / inválida não autentica.
	videoID := "550e8400-e29b-41d4-a716-446655440021"
	body := []byte(`{"video_id":"` + videoID + `","declared_size_bytes":2048}`)
	req := fazRequestInitComProjectKey("chave-forjada-nao-pertence-a-projeto-algum", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperava 401 para chave de projeto inválida, obteve %d: %s", rec.Code, rec.Body.String())
	}

	// A chave do Projeto B autentica normalmente — apenas para si mesma — e
	// o vídeo/token ficam vinculados ao Projeto B (não a outro projeto).
	req2 := fazRequestInitComProjectKey(masterKeyB, body)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("esperava 200 para chave do Projeto B, obteve %d: %s", rec2.Code, rec2.Body.String())
	}

	projectB, err := models.GetProjectBySlug(database, "projeto-b")
	if err != nil {
		t.Fatalf("GetProjectBySlug: %v", err)
	}
	video, err := models.GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo: %v", err)
	}
	if video.ProjectID == nil || *video.ProjectID != projectB.ID {
		t.Errorf("vídeo deveria estar vinculado ao Projeto B (%d), obteve %v", projectB.ID, video.ProjectID)
	}
}
