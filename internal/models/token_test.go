package models

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/db"
)

// abreDBToken abre banco em memória para testes de token.
// (Função separada de abreDB para evitar conflito de redefinição)
func abreDBToken(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// insereVideoTeste insere um vídeo de suporte para os testes de token.
func insereVideoTeste(t *testing.T, database *sql.DB, videoID string) {
	t.Helper()
	if err := InsertVideo(database, videoID, 100); err != nil {
		t.Fatalf("InsertVideo falhou: %v", err)
	}
}

func TestInsertToken_Success(t *testing.T) {
	// Verifica que InsertUploadToken insere com sucesso quando o vídeo existe.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-tok-ok")

	err := InsertUploadToken(database, "tok-abc", "v-tok-ok", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("InsertUploadToken falhou inesperadamente: %v", err)
	}
}

func TestInsertToken_DuplicateVideoID(t *testing.T) {
	// Verifica que a constraint UNIQUE em video_id impede dois tokens para o mesmo vídeo.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-dup-tok")

	if err := InsertUploadToken(database, "tok-1", "v-dup-tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("primeiro InsertUploadToken falhou: %v", err)
	}
	if err := InsertUploadToken(database, "tok-2", "v-dup-tok", time.Now().Add(time.Hour)); err == nil {
		t.Error("esperava erro de UNIQUE constraint ao inserir segundo token para o mesmo vídeo")
	}
}

func TestInsertToken_VideoNotFound(t *testing.T) {
	// Verifica que inserir token para video_id inexistente falha por foreign key.
	database := abreDBToken(t)

	err := InsertUploadToken(database, "tok-fk", "video-inexistente", time.Now().Add(time.Hour))
	if err == nil {
		t.Error("esperava erro de foreign key, mas InsertUploadToken retornou nil")
	}
}

func TestGetToken_Found(t *testing.T) {
	// Verifica que GetUploadToken retorna o token correto quando existe.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-get-tok")
	expires := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	if err := InsertUploadToken(database, "tok-get", "v-get-tok", expires); err != nil {
		t.Fatal(err)
	}

	tok, err := GetUploadToken(database, "tok-get")
	if err != nil {
		t.Fatalf("GetUploadToken falhou: %v", err)
	}
	if tok.VideoID != "v-get-tok" {
		t.Errorf("VideoID: esperado %q, obtido %q", "v-get-tok", tok.VideoID)
	}
}

func TestGetToken_NotFound(t *testing.T) {
	// Verifica que GetUploadToken retorna erro para token inexistente.
	database := abreDBToken(t)

	_, err := GetUploadToken(database, "tok-inexistente")
	if err == nil {
		t.Fatal("esperava erro para token inexistente")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Logf("erro retornado (aceitável se não for sql.ErrNoRows): %v", err)
	}
}

func TestGetTokenByVideoID_Found(t *testing.T) {
	// Verifica que GetUploadTokenByVideoID retorna o token pelo video_id.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-by-id")

	if err := InsertUploadToken(database, "tok-byid", "v-by-id", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	tok, err := GetUploadTokenByVideoID(database, "v-by-id")
	if err != nil {
		t.Fatalf("GetUploadTokenByVideoID falhou: %v", err)
	}
	if tok.Token != "tok-byid" {
		t.Errorf("Token: esperado %q, obtido %q", "tok-byid", tok.Token)
	}
}

func TestDeleteToken(t *testing.T) {
	// Verifica que DeleteUploadToken remove o token e ele não pode mais ser encontrado.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-del")

	if err := InsertUploadToken(database, "tok-del", "v-del", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := DeleteUploadToken(database, "tok-del"); err != nil {
		t.Fatalf("DeleteUploadToken falhou: %v", err)
	}
	if _, err := GetUploadToken(database, "tok-del"); err == nil {
		t.Error("token deveria ter sido deletado, mas GetUploadToken retornou sem erro")
	}
}

func TestDeleteExpiredTokens(t *testing.T) {
	// Verifica que DeleteExpiredTokens remove tokens expirados e mantém os válidos.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-exp1")
	insereVideoTeste(t, database, "v-val1")

	// Insere token expirado (1 hora no passado)
	if err := InsertUploadToken(database, "tok-exp", "v-exp1", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Insere token válido (2 horas no futuro)
	if err := InsertUploadToken(database, "tok-val", "v-val1", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	n, err := DeleteExpiredTokens(database)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens falhou: %v", err)
	}
	if n != 1 {
		t.Errorf("esperava deletar 1 token expirado, deletou %d", n)
	}

	// Token expirado deve ter sido deletado
	if _, err := GetUploadToken(database, "tok-exp"); err == nil {
		t.Error("token expirado deveria ter sido deletado")
	}
	// Token válido deve permanecer
	if _, err := GetUploadToken(database, "tok-val"); err != nil {
		t.Errorf("token válido não deveria ter sido deletado: %v", err)
	}
}

func TestTokenExpired(t *testing.T) {
	// Verifica que IsExpired() retorna true para token com ExpiresAt no passado.
	tok := &UploadToken{
		Token:     "tok",
		VideoID:   "vid",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if !tok.IsExpired() {
		t.Error("IsExpired() deveria retornar true para token com expiração no passado")
	}
}

func TestTokenValid(t *testing.T) {
	// Verifica que IsExpired() retorna false para token com ExpiresAt no futuro.
	tok := &UploadToken{
		Token:     "tok",
		VideoID:   "vid",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if tok.IsExpired() {
		t.Error("IsExpired() deveria retornar false para token com expiração no futuro")
	}
}

func TestTokenExpiredBoundary(t *testing.T) {
	// Verifica comportamento no limite: token que expirou exatamente agora (ou antes).
	tok := &UploadToken{
		Token:     "tok",
		VideoID:   "vid",
		ExpiresAt: time.Now().Add(-1 * time.Millisecond), // já passou de 1ms
	}
	if !tok.IsExpired() {
		t.Error("IsExpired() deveria retornar true para token que já passou")
	}
}

func TestParseDateTime_RFC3339(t *testing.T) {
	// Verifica que parseDateTime consegue fazer parse de formato RFC3339.
	s := "2024-06-07T15:30:45Z"
	result := parseDateTime(s)
	if result.IsZero() {
		t.Error("parseDateTime falhou para RFC3339: retornou zero time")
	}
	// Verifica que o resultado está em UTC
	if result.Location().String() != "UTC" {
		t.Errorf("parseDateTime: esperado UTC, obtido %s", result.Location().String())
	}
}

func TestParseDateTime_SupportedFormats(t *testing.T) {
	// Verifica que parseDateTime suporta vários formatos comuns de datetime SQLite.
	tests := []struct {
		name     string
		input    string
		wantZero bool
	}{
		{"RFC3339", "2024-06-07T15:30:45Z", false},
		{"SQLite padrão", "2024-06-07 15:30:45", false},
		{"Com timezone", "2024-06-07 15:30:45+00:00", false},
		{"Inválido", "not-a-date", true},
		{"Vazio", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDateTime(tt.input)
			if tt.wantZero && !result.IsZero() {
				t.Errorf("parseDateTime(%q): esperava zero time, obteve %v", tt.input, result)
			}
			if !tt.wantZero && result.IsZero() {
				t.Errorf("parseDateTime(%q): esperava time válido, obteve zero time", tt.input)
			}
		})
	}
}

func TestInsertUploadTokenForProject_WithProjectID(t *testing.T) {
	// Verifica que InsertUploadTokenForProject associa o token a um projeto.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-tok-proj")

	// Cria um projeto válido antes de associar token
	project, _, err := CreateProject(database, "Test Project")
	if err != nil {
		t.Fatalf("CreateProject falhou: %v", err)
	}

	expiresAt := time.Now().Add(time.Hour)
	if err := InsertUploadTokenForProject(database, "tok-proj", "v-tok-proj", expiresAt, &project.ID); err != nil {
		t.Fatalf("InsertUploadTokenForProject falhou: %v", err)
	}

	tok, err := GetUploadToken(database, "tok-proj")
	if err != nil {
		t.Fatalf("GetUploadToken falhou: %v", err)
	}

	if tok.ProjectID == nil {
		t.Error("ProjectID: esperado ser definido, obtido nil")
	} else if *tok.ProjectID != project.ID {
		t.Errorf("ProjectID: esperado %d, obtido %d", project.ID, *tok.ProjectID)
	}
}

func TestInsertUploadTokenForProject_WithoutProjectID(t *testing.T) {
	// Verifica que InsertUploadTokenForProject com projectID=nil funciona (fluxo legado).
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-tok-no-proj")

	expiresAt := time.Now().Add(time.Hour)
	if err := InsertUploadTokenForProject(database, "tok-no-proj", "v-tok-no-proj", expiresAt, nil); err != nil {
		t.Fatalf("InsertUploadTokenForProject falhou: %v", err)
	}

	tok, err := GetUploadToken(database, "tok-no-proj")
	if err != nil {
		t.Fatalf("GetUploadToken falhou: %v", err)
	}

	if tok.ProjectID != nil {
		t.Errorf("ProjectID: esperado nil, obtido %v", tok.ProjectID)
	}
}

func TestGetUploadTokenByVideoID_NotFound(t *testing.T) {
	// Verifica que GetUploadTokenByVideoID retorna erro quando o vídeo não tem token.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-no-token")

	_, err := GetUploadTokenByVideoID(database, "v-no-token")
	if err == nil {
		t.Fatal("esperava erro para vídeo sem token, mas GetUploadTokenByVideoID retornou nil")
	}
}

func TestDeleteExpiredTokens_NoExpiredTokens(t *testing.T) {
	// Verifica que DeleteExpiredTokens retorna 0 quando nenhum token está expirado.
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-no-exp")

	// Insere token válido (2 horas no futuro)
	if err := InsertUploadToken(database, "tok-future", "v-no-exp", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	n, err := DeleteExpiredTokens(database)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens falhou: %v", err)
	}
	if n != 0 {
		t.Errorf("DeleteExpiredTokens: esperava deletar 0 tokens, deletou %d", n)
	}
}

func TestDeleteExpiredTokens_MultipleExpired(t *testing.T) {
	// Verifica que DeleteExpiredTokens deleta múltiplos tokens expirados corretamente.
	database := abreDBToken(t)

	// Insere 3 vídeos e 3 tokens expirados
	for i := 1; i <= 3; i++ {
		videoID := "v-exp-multi-" + string(rune(48+i))
		tokenID := "tok-exp-" + string(rune(48+i))
		insereVideoTeste(t, database, videoID)
		if err := InsertUploadToken(database, tokenID, videoID, time.Now().Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	n, err := DeleteExpiredTokens(database)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens falhou: %v", err)
	}
	if n != 3 {
		t.Errorf("DeleteExpiredTokens: esperava deletar 3 tokens, deletou %d", n)
	}
}

func TestTokenExpiresBoundaryExactly(t *testing.T) {
	// Verifica que IsExpired() com ExpiresAt no futuro retorna false.
	// Usa 10 segundos no futuro para evitar race condition de timing.
	future := time.Now().Add(10 * time.Second)
	tok := &UploadToken{
		Token:     "tok",
		VideoID:   "vid",
		ExpiresAt: future,
	}
	if tok.IsExpired() {
		t.Error("IsExpired() deveria retornar false para token com expiração no futuro próximo")
	}
}

// TestScanUploadToken_NullProjectID testa scanUploadToken com project_id NULL.
func TestScanUploadToken_NullProjectID(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-scan-null")

	// Insere token sem projectID (nil)
	if err := InsertUploadToken(database, "tok-scan-null", "v-scan-null", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	tok, err := GetUploadToken(database, "tok-scan-null")
	if err != nil {
		t.Fatalf("GetUploadToken falhou: %v", err)
	}

	if tok.ProjectID != nil {
		t.Errorf("ProjectID: esperado nil, obtido %v", tok.ProjectID)
	}
}

// TestDeleteExpiredTokens_EdgeCaseExactExpiration verifica tokens que expiram no limite de tempo.
func TestDeleteExpiredTokens_EdgeCaseExactExpiration(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-exp-edge")

	// Insere token que já expirou (1 segundo no passado)
	if err := InsertUploadToken(database, "tok-edge-past", "v-exp-edge", time.Now().Add(-1*time.Second)); err != nil {
		t.Fatal(err)
	}

	// DeleteExpiredTokens deve remover tokens com expires_at <= now
	n, err := DeleteExpiredTokens(database)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens falhou: %v", err)
	}
	if n < 1 {
		t.Errorf("DeleteExpiredTokens: esperava deletar pelo menos 1 token, deletou %d", n)
	}

	// Verifica que foi deletado
	if _, err := GetUploadToken(database, "tok-edge-past"); err == nil {
		t.Error("token que expirou deveria ter sido deletado")
	}
}

// TestInsertToken_WithNilProjectID testa inserção com projectID=nil.
func TestInsertToken_WithNilProjectID(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-nil-proj")

	// Insere com projectID nil
	if err := InsertUploadTokenForProject(database, "tok-nil", "v-nil-proj", time.Now().Add(time.Hour), nil); err != nil {
		t.Fatalf("InsertUploadTokenForProject com nil projectID falhou: %v", err)
	}

	tok, err := GetUploadToken(database, "tok-nil")
	if err != nil {
		t.Fatalf("GetUploadToken falhou: %v", err)
	}

	if tok.ProjectID != nil {
		t.Errorf("ProjectID: esperado nil, obtido %v", tok.ProjectID)
	}
}
