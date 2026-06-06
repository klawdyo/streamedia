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
