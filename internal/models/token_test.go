package models

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/db"
)

// abreDBToken abre banco em memória para testes de token.
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

func TestInsertAccessToken_Success(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-tok-ok")

	err := InsertAccessToken(database, "tok-abc", "v-tok-ok", PurposeUpload, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("InsertAccessToken falhou inesperadamente: %v", err)
	}
}

func TestInsertAccessToken_ReplacesSamePurpose(t *testing.T) {
	// UNIQUE(video_id, purpose) + INSERT OR REPLACE: reemitir um token do mesmo
	// (vídeo, propósito) substitui o anterior (rotação — o antigo morre).
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-rot")

	if err := InsertAccessToken(database, "tok-1", "v-rot", PurposePlay, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("primeiro InsertAccessToken falhou: %v", err)
	}
	if err := InsertAccessToken(database, "tok-2", "v-rot", PurposePlay, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("segundo InsertAccessToken (replace) falhou: %v", err)
	}
	// O antigo deve ter sumido; o novo deve existir.
	if _, err := GetAccessToken(database, "tok-1"); err == nil {
		t.Error("token antigo deveria ter sido substituído (rotação)")
	}
	if _, err := GetAccessToken(database, "tok-2"); err != nil {
		t.Errorf("token novo deveria existir: %v", err)
	}
}

func TestInsertAccessToken_BothPurposesCoexist(t *testing.T) {
	// Upload e play podem coexistir para o mesmo vídeo (purposes distintos).
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-both")

	if err := InsertAccessToken(database, "tok-up", "v-both", PurposeUpload, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := InsertAccessToken(database, "tok-pl", "v-both", PurposePlay, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("play token deveria coexistir com upload token: %v", err)
	}
}

func TestInsertAccessToken_VideoNotFound(t *testing.T) {
	database := abreDBToken(t)
	err := InsertAccessToken(database, "tok-fk", "video-inexistente", PurposeUpload, time.Now().Add(time.Hour))
	if err == nil {
		t.Error("esperava erro de foreign key, mas InsertAccessToken retornou nil")
	}
}

func TestGetAccessToken_FoundWithPurpose(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-get-tok")
	expires := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	if err := InsertAccessToken(database, "tok-get", "v-get-tok", PurposePlay, expires); err != nil {
		t.Fatal(err)
	}

	tok, err := GetAccessToken(database, "tok-get")
	if err != nil {
		t.Fatalf("GetAccessToken falhou: %v", err)
	}
	if tok.VideoID != "v-get-tok" {
		t.Errorf("VideoID: esperado %q, obtido %q", "v-get-tok", tok.VideoID)
	}
	if tok.Purpose != PurposePlay {
		t.Errorf("Purpose: esperado %q, obtido %q", PurposePlay, tok.Purpose)
	}
}

func TestGetAccessToken_NotFound(t *testing.T) {
	database := abreDBToken(t)
	_, err := GetAccessToken(database, "tok-inexistente")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("esperava sql.ErrNoRows para token inexistente, obteve %v", err)
	}
}

func TestDeleteAccessTokensForVideo(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-del")
	if err := InsertAccessToken(database, "tok-u", "v-del", PurposeUpload, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := InsertAccessToken(database, "tok-p", "v-del", PurposePlay, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := DeleteAccessTokensForVideo(database, "v-del"); err != nil {
		t.Fatalf("DeleteAccessTokensForVideo falhou: %v", err)
	}
	if _, err := GetAccessToken(database, "tok-u"); err == nil {
		t.Error("tok-u deveria ter sido deletado")
	}
	if _, err := GetAccessToken(database, "tok-p"); err == nil {
		t.Error("tok-p deveria ter sido deletado")
	}
}

func TestDeleteExpiredTokens(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-exp1")
	insereVideoTeste(t, database, "v-val1")

	if err := InsertAccessToken(database, "tok-exp", "v-exp1", PurposeUpload, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := InsertAccessToken(database, "tok-val", "v-val1", PurposeUpload, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	n, err := DeleteExpiredTokens(database)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens falhou: %v", err)
	}
	if n != 1 {
		t.Errorf("esperava deletar 1 token expirado, deletou %d", n)
	}
	if _, err := GetAccessToken(database, "tok-exp"); err == nil {
		t.Error("token expirado deveria ter sido deletado")
	}
	if _, err := GetAccessToken(database, "tok-val"); err != nil {
		t.Errorf("token válido não deveria ter sido deletado: %v", err)
	}
}

func TestDeleteExpiredTokens_NoExpiredTokens(t *testing.T) {
	database := abreDBToken(t)
	insereVideoTeste(t, database, "v-no-exp")
	if err := InsertAccessToken(database, "tok-future", "v-no-exp", PurposePlay, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	n, err := DeleteExpiredTokens(database)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens falhou: %v", err)
	}
	if n != 0 {
		t.Errorf("esperava deletar 0 tokens, deletou %d", n)
	}
}

func TestDeleteExpiredTokens_MultipleExpired(t *testing.T) {
	database := abreDBToken(t)
	for i := 1; i <= 3; i++ {
		videoID := "v-exp-multi-" + string(rune(48+i))
		tokenID := "tok-exp-" + string(rune(48+i))
		insereVideoTeste(t, database, videoID)
		if err := InsertAccessToken(database, tokenID, videoID, PurposeUpload, time.Now().Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	n, err := DeleteExpiredTokens(database)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens falhou: %v", err)
	}
	if n != 3 {
		t.Errorf("esperava deletar 3 tokens, deletou %d", n)
	}
}

func TestAccessTokenExpired(t *testing.T) {
	tok := &AccessToken{Token: "tok", VideoID: "vid", Purpose: PurposeUpload, ExpiresAt: time.Now().Add(-time.Minute)}
	if !tok.IsExpired() {
		t.Error("IsExpired() deveria retornar true para token com expiração no passado")
	}
}

func TestAccessTokenValid(t *testing.T) {
	tok := &AccessToken{Token: "tok", VideoID: "vid", Purpose: PurposePlay, ExpiresAt: time.Now().Add(time.Hour)}
	if tok.IsExpired() {
		t.Error("IsExpired() deveria retornar false para token com expiração no futuro")
	}
}

func TestParseDateTime_RFC3339(t *testing.T) {
	result := parseDateTime("2024-06-07T15:30:45Z")
	if result.IsZero() {
		t.Error("parseDateTime falhou para RFC3339: retornou zero time")
	}
	if result.Location().String() != "UTC" {
		t.Errorf("parseDateTime: esperado UTC, obtido %s", result.Location().String())
	}
}

func TestParseDateTime_SupportedFormats(t *testing.T) {
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
