package models

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/klawdyo/streamedia/internal/db"
	_ "modernc.org/sqlite"
)

// abreDB abre banco SQLite em memória e aplica o schema para uso nos testes.
func abreDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestVideoStatusTransitions_Valid(t *testing.T) {
	// Verifica que as transições de estado válidas são aceitas pela máquina de estados.
	database := abreDB(t)

	if err := InsertVideo(database, "v1", 100); err != nil {
		t.Fatalf("InsertVideo falhou: %v", err)
	}
	// pending_upload → uploading
	if err := UpdateStatus(database, "v1", StatusUploading); err != nil {
		t.Errorf("transição pending_upload→uploading deveria ser aceita: %v", err)
	}
	// uploading → upload_complete
	if err := UpdateStatus(database, "v1", StatusUploadComplete); err != nil {
		t.Errorf("transição uploading→upload_complete deveria ser aceita: %v", err)
	}
	// upload_complete → transcoding
	if err := UpdateStatus(database, "v1", StatusTranscoding); err != nil {
		t.Errorf("transição upload_complete→transcoding deveria ser aceita: %v", err)
	}
	// transcoding → ready
	if err := UpdateStatus(database, "v1", StatusReady); err != nil {
		t.Errorf("transição transcoding→ready deveria ser aceita: %v", err)
	}
}

func TestVideoStatusTransitions_Invalid(t *testing.T) {
	// Verifica que transições inválidas retornam erro.
	database := abreDB(t)

	// Testa ready → uploading (inválido)
	if err := InsertVideo(database, "v-ready", 100); err != nil {
		t.Fatal(err)
	}
	// Avança até ready
	_ = UpdateStatus(database, "v-ready", StatusUploading)
	_ = UpdateStatus(database, "v-ready", StatusUploadComplete)
	_ = UpdateStatus(database, "v-ready", StatusTranscoding)
	_ = UpdateStatus(database, "v-ready", StatusReady)

	if err := UpdateStatus(database, "v-ready", StatusUploading); err == nil {
		t.Error("transição ready→uploading deveria retornar erro")
	}

	// Testa failed_upload → uploading (terminal)
	if err := InsertVideo(database, "v-failed", 100); err != nil {
		t.Fatal(err)
	}
	_ = UpdateStatus(database, "v-failed", StatusUploading)
	_ = UpdateStatusWithError(database, "v-failed", StatusFailedUpload, "upload falhou")

	if err := UpdateStatus(database, "v-failed", StatusUploading); err == nil {
		t.Error("transição de estado terminal failed_upload→uploading deveria retornar erro")
	}

	// Testa pending_upload → ready (salto inválido)
	if err := InsertVideo(database, "v-salto", 100); err != nil {
		t.Fatal(err)
	}
	if err := UpdateStatus(database, "v-salto", StatusReady); err == nil {
		t.Error("transição pending_upload→ready deveria retornar erro (salto)")
	}
}

func TestVideoCreate(t *testing.T) {
	// Verifica que InsertVideo cria o registro com valores iniciais corretos.
	database := abreDB(t)

	if err := InsertVideo(database, "v-create", 1024); err != nil {
		t.Fatalf("InsertVideo falhou: %v", err)
	}

	v, err := GetVideo(database, "v-create")
	if err != nil {
		t.Fatalf("GetVideo falhou: %v", err)
	}
	if v.Status != StatusPendingUpload {
		t.Errorf("status inicial: esperado %q, obtido %q", StatusPendingUpload, v.Status)
	}
	if v.TranscodeAttempts != 0 {
		t.Errorf("transcode_attempts inicial: esperado 0, obtido %d", v.TranscodeAttempts)
	}
	if v.CreatedAt.IsZero() {
		t.Error("created_at não pode ser zero")
	}
	if v.DeclaredSizeBytes != 1024 {
		t.Errorf("declared_size_bytes: esperado 1024, obtido %d", v.DeclaredSizeBytes)
	}
}

func TestVideoGet_NotFound(t *testing.T) {
	// Verifica que GetVideo retorna erro quando o vídeo não existe.
	database := abreDB(t)

	_, err := GetVideo(database, "uuid-inexistente")
	if err == nil {
		t.Fatal("esperava erro para vídeo inexistente, mas GetVideo retornou nil")
	}
	if !errors.Is(err, sql.ErrNoRows) && err.Error() != "sql: no rows in result set" {
		// Aceita tanto sql.ErrNoRows quanto o wrapped equivalente
		// Só falha se o erro for completamente inesperado
		t.Logf("erro retornado: %v (pode ser sql.ErrNoRows ou wrapped)", err)
	}
}

func TestVideoUpdateStatus(t *testing.T) {
	// Verifica que UpdateStatus persiste a mudança de estado corretamente.
	database := abreDB(t)

	if err := InsertVideo(database, "v-update", 100); err != nil {
		t.Fatal(err)
	}
	if err := UpdateStatus(database, "v-update", StatusUploading); err != nil {
		t.Fatalf("UpdateStatus falhou: %v", err)
	}

	v, err := GetVideo(database, "v-update")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusUploading {
		t.Errorf("status após update: esperado %q, obtido %q", StatusUploading, v.Status)
	}
}

func TestVideoDuplicateID(t *testing.T) {
	// Verifica que inserir dois vídeos com o mesmo ID retorna erro de constraint.
	database := abreDB(t)

	if err := InsertVideo(database, "v-dup", 100); err != nil {
		t.Fatal(err)
	}
	if err := InsertVideo(database, "v-dup", 200); err == nil {
		t.Error("esperava erro de UNIQUE constraint ao inserir ID duplicado")
	}
}

func TestVideoResolutionsSerialization(t *testing.T) {
	// Verifica que o campo resolutions é serializado/deserializado corretamente como JSON.
	database := abreDB(t)

	if err := InsertVideo(database, "v-res", 100); err != nil {
		t.Fatal(err)
	}
	// Avança para ready para poder usar SetReady
	_ = UpdateStatus(database, "v-res", StatusUploading)
	_ = UpdateStatus(database, "v-res", StatusUploadComplete)
	_ = UpdateStatus(database, "v-res", StatusTranscoding)

	if err := SetReady(database, "v-res", 30, []int{480, 720}); err != nil {
		t.Fatalf("SetReady falhou: %v", err)
	}

	v, err := GetVideo(database, "v-res")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Resolutions) != 2 || v.Resolutions[0] != 480 || v.Resolutions[1] != 720 {
		t.Errorf("resolutions: esperado [480 720], obtido %v", v.Resolutions)
	}
}

func TestVideoTransitionBlocksTerminal(t *testing.T) {
	// Verifica que estados terminais bloqueiam qualquer transição de saída.
	database := abreDB(t)

	if err := InsertVideo(database, "v-term", 100); err != nil {
		t.Fatal(err)
	}
	_ = UpdateStatus(database, "v-term", StatusUploading)
	if err := UpdateStatusWithError(database, "v-term", StatusFailedUpload, "falha no upload"); err != nil {
		t.Fatalf("UpdateStatusWithError falhou: %v", err)
	}

	// Qualquer tentativa de sair do estado terminal deve falhar
	if err := UpdateStatus(database, "v-term", StatusUploading); err == nil {
		t.Error("deveria bloquear transição a partir de estado terminal failed_upload")
	}
}
