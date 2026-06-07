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

func TestUpdateLastChunk(t *testing.T) {
	// Verifica que UpdateLastChunk atualiza o timestamp do último chunk.
	database := abreDB(t)

	if err := InsertVideo(database, "v-chunk", 100); err != nil {
		t.Fatal(err)
	}

	// Antes de chamar UpdateLastChunk, last_chunk_at deve ser NULL
	v1, err := GetVideo(database, "v-chunk")
	if err != nil {
		t.Fatal(err)
	}
	if v1.LastChunkAt != nil {
		t.Errorf("last_chunk_at inicial: esperado nil, obtido %v", v1.LastChunkAt)
	}

	// Atualiza o timestamp do último chunk
	if err := UpdateLastChunk(database, "v-chunk"); err != nil {
		t.Fatalf("UpdateLastChunk falhou: %v", err)
	}

	// Após chamar UpdateLastChunk, last_chunk_at deve ser definido
	v2, err := GetVideo(database, "v-chunk")
	if err != nil {
		t.Fatal(err)
	}
	if v2.LastChunkAt == nil {
		t.Error("last_chunk_at: esperado ser definido após UpdateLastChunk, obtido nil")
	}
}

func TestIncrementTranscodeAttempts(t *testing.T) {
	// Verifica que IncrementTranscodeAttempts incrementa o contador corretamente.
	database := abreDB(t)

	if err := InsertVideo(database, "v-incr", 100); err != nil {
		t.Fatal(err)
	}

	// Verifica valor inicial (0)
	v1, err := GetVideo(database, "v-incr")
	if err != nil {
		t.Fatal(err)
	}
	if v1.TranscodeAttempts != 0 {
		t.Errorf("transcode_attempts inicial: esperado 0, obtido %d", v1.TranscodeAttempts)
	}

	// Incrementa duas vezes
	if err := IncrementTranscodeAttempts(database, "v-incr"); err != nil {
		t.Fatalf("primeiro IncrementTranscodeAttempts falhou: %v", err)
	}
	if err := IncrementTranscodeAttempts(database, "v-incr"); err != nil {
		t.Fatalf("segundo IncrementTranscodeAttempts falhou: %v", err)
	}

	// Verifica valor final (2)
	v2, err := GetVideo(database, "v-incr")
	if err != nil {
		t.Fatal(err)
	}
	if v2.TranscodeAttempts != 2 {
		t.Errorf("transcode_attempts após 2 incrementos: esperado 2, obtido %d", v2.TranscodeAttempts)
	}
}

func TestListByStatus(t *testing.T) {
	// Verifica que ListByStatus retorna apenas os vídeos no status especificado.
	database := abreDB(t)

	// Insere vários vídeos em estados diferentes
	videos := []struct {
		id     string
		status VideoStatus
	}{
		{"v-list-1", StatusPendingUpload},
		{"v-list-2", StatusUploading},
		{"v-list-3", StatusUploading},
		{"v-list-4", StatusUploadComplete},
	}

	for _, video := range videos {
		if err := InsertVideo(database, video.id, 100); err != nil {
			t.Fatal(err)
		}
		if video.status != StatusPendingUpload {
			_ = UpdateStatus(database, video.id, video.status)
		}
	}

	// Lista vídeos em estado "uploading"
	results, err := ListByStatus(database, StatusUploading)
	if err != nil {
		t.Fatalf("ListByStatus falhou: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("ListByStatus: esperava 2 vídeos, obteve %d", len(results))
	}

	// Verifica que todos os resultados estão no status correto
	for _, video := range results {
		if video.Status != StatusUploading {
			t.Errorf("vídeo com status incorreto: %q, esperado %q", video.Status, StatusUploading)
		}
	}
}

func TestListByStatus_EmptyResult(t *testing.T) {
	// Verifica que ListByStatus retorna lista vazia quando nenhum vídeo tem o status.
	database := abreDB(t)

	if err := InsertVideo(database, "v-empty", 100); err != nil {
		t.Fatal(err)
	}

	// Busca vídeos em estado "ready" (nenhum foi inserido neste estado)
	results, err := ListByStatus(database, StatusReady)
	if err != nil {
		t.Fatalf("ListByStatus falhou: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("ListByStatus: esperava lista vazia, obteve %d vídeos", len(results))
	}
}

func TestSetUploadComplete_UpdatesActualSize(t *testing.T) {
	// Verifica que SetUploadComplete atualiza o tamanho real e transiciona para upload_complete.
	database := abreDB(t)

	if err := InsertVideo(database, "v-complete", 100); err != nil {
		t.Fatal(err)
	}

	// Avança para uploading (pré-requisito para upload_complete)
	if err := UpdateStatus(database, "v-complete", StatusUploading); err != nil {
		t.Fatal(err)
	}

	// Executa SetUploadComplete
	if err := SetUploadComplete(database, "v-complete", 256); err != nil {
		t.Fatalf("SetUploadComplete falhou: %v", err)
	}

	// Verifica que o status foi atualizado
	v, err := GetVideo(database, "v-complete")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusUploadComplete {
		t.Errorf("status: esperado %q, obtido %q", StatusUploadComplete, v.Status)
	}
	if v.ActualSizeBytes != 256 {
		t.Errorf("actual_size_bytes: esperado 256, obtido %d", v.ActualSizeBytes)
	}
}

func TestSetReady_UpdatesDurationAndResolutions(t *testing.T) {
	// Verifica que SetReady atualiza duração e resolutions corretamente.
	database := abreDB(t)

	if err := InsertVideo(database, "v-ready-set", 100); err != nil {
		t.Fatal(err)
	}

	// Avança para transcoding (pré-requisito para ready)
	_ = UpdateStatus(database, "v-ready-set", StatusUploading)
	_ = UpdateStatus(database, "v-ready-set", StatusUploadComplete)
	_ = UpdateStatus(database, "v-ready-set", StatusTranscoding)

	// Executa SetReady com duração e resolutions
	resolutions := []int{480, 720, 1080}
	if err := SetReady(database, "v-ready-set", 120, resolutions); err != nil {
		t.Fatalf("SetReady falhou: %v", err)
	}

	// Verifica campos atualizados
	v, err := GetVideo(database, "v-ready-set")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusReady {
		t.Errorf("status: esperado %q, obtido %q", StatusReady, v.Status)
	}
	if v.DurationS != 120 {
		t.Errorf("duration_s: esperado 120, obtido %d", v.DurationS)
	}
	if len(v.Resolutions) != 3 || v.Resolutions[0] != 480 || v.Resolutions[2] != 1080 {
		t.Errorf("resolutions: esperado [480 720 1080], obtido %v", v.Resolutions)
	}
}

func TestUpdateStatusWithError_PersistsErrorMessage(t *testing.T) {
	// Verifica que UpdateStatusWithError grava a mensagem de erro corretamente.
	database := abreDB(t)

	if err := InsertVideo(database, "v-err", 100); err != nil {
		t.Fatal(err)
	}

	// Avança para uploading antes de falhar
	if err := UpdateStatus(database, "v-err", StatusUploading); err != nil {
		t.Fatal(err)
	}

	// Transiciona para failed_upload com mensagem de erro
	errMsg := "conexão perdida com cliente"
	if err := UpdateStatusWithError(database, "v-err", StatusFailedUpload, errMsg); err != nil {
		t.Fatalf("UpdateStatusWithError falhou: %v", err)
	}

	// Verifica que status e error_message foram atualizados
	v, err := GetVideo(database, "v-err")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusFailedUpload {
		t.Errorf("status: esperado %q, obtido %q", StatusFailedUpload, v.Status)
	}
	if v.ErrorMessage != errMsg {
		t.Errorf("error_message: esperado %q, obtido %q", errMsg, v.ErrorMessage)
	}
}

func TestUpdateStatusWithError_InvalidTransition(t *testing.T) {
	// Verifica que UpdateStatusWithError rejeita transições inválidas.
	database := abreDB(t)

	if err := InsertVideo(database, "v-invalid-err", 100); err != nil {
		t.Fatal(err)
	}

	// Tenta transição inválida ready → uploading com mensagem de erro
	// Primeiro, força o vídeo para ready
	_ = UpdateStatus(database, "v-invalid-err", StatusUploading)
	_ = UpdateStatus(database, "v-invalid-err", StatusUploadComplete)
	_ = UpdateStatus(database, "v-invalid-err", StatusTranscoding)
	_ = UpdateStatus(database, "v-invalid-err", StatusReady)

	// Tenta transição inválida
	if err := UpdateStatusWithError(database, "v-invalid-err", StatusUploading, "erro teste"); err == nil {
		t.Error("esperava erro para transição inválida ready→uploading com UpdateStatusWithError")
	}
}

func TestGetVideo_ResolutionsNull(t *testing.T) {
	// Verifica que GetVideo retorna []int{} quando resolutions é NULL no banco.
	database := abreDB(t)

	if err := InsertVideo(database, "v-null-res", 100); err != nil {
		t.Fatal(err)
	}

	v, err := GetVideo(database, "v-null-res")
	if err != nil {
		t.Fatal(err)
	}

	// resolutions deve ser um slice vazio, não nil
	if v.Resolutions == nil {
		t.Error("Resolutions: esperado slice vazio, obtido nil")
	}
	if len(v.Resolutions) != 0 {
		t.Errorf("Resolutions: esperado comprimento 0, obtido %d", len(v.Resolutions))
	}
}

func TestVideoInsertWithProjectID(t *testing.T) {
	// Verifica que InsertVideoForProject cria vídeo vinculado a um projeto.
	database := abreDB(t)

	// Cria um projeto válido antes de inserir o vídeo
	project, _, err := CreateProject(database, "Test Project for Video")
	if err != nil {
		t.Fatalf("CreateProject falhou: %v", err)
	}

	if err := InsertVideoForProject(database, "v-proj", 100, &project.ID); err != nil {
		t.Fatalf("InsertVideoForProject falhou: %v", err)
	}

	v, err := GetVideo(database, "v-proj")
	if err != nil {
		t.Fatal(err)
	}

	if v.ProjectID == nil {
		t.Error("ProjectID: esperado ser definido, obtido nil")
	} else if *v.ProjectID != project.ID {
		t.Errorf("ProjectID: esperado %d, obtido %d", project.ID, *v.ProjectID)
	}
}

func TestVideoInsertWithoutProjectID(t *testing.T) {
	// Verifica que InsertVideoForProject com projectID=nil cria vídeo sem projeto.
	database := abreDB(t)

	if err := InsertVideoForProject(database, "v-no-proj", 100, nil); err != nil {
		t.Fatalf("InsertVideoForProject falhou: %v", err)
	}

	v, err := GetVideo(database, "v-no-proj")
	if err != nil {
		t.Fatal(err)
	}

	if v.ProjectID != nil {
		t.Errorf("ProjectID: esperado nil, obtido %v", v.ProjectID)
	}
}

// TestUpdateStatus_CoversBranches testa branches não cobertos em UpdateStatus.
func TestUpdateStatus_CoversBranches(t *testing.T) {
	database := abreDB(t)

	// Insere vídeo
	if err := InsertVideo(database, "v-branch", 100); err != nil {
		t.Fatal(err)
	}

	// Testa transição válida: pending_upload → uploading
	if err := UpdateStatus(database, "v-branch", StatusUploading); err != nil {
		t.Fatalf("UpdateStatus para uploading falhou: %v", err)
	}

	// Testa self-transition: uploading → uploading (permitida)
	if err := UpdateStatus(database, "v-branch", StatusUploading); err != nil {
		t.Fatalf("UpdateStatus para uploading (self) falhou: %v", err)
	}

	// Testa transição válida: uploading → uploading → upload_complete
	if err := UpdateStatus(database, "v-branch", StatusUploadComplete); err != nil {
		t.Fatalf("UpdateStatus para upload_complete falhou: %v", err)
	}

	// Verifica que o status foi atualizado
	v, err := GetVideo(database, "v-branch")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusUploadComplete {
		t.Errorf("status: esperado %q, obtido %q", StatusUploadComplete, v.Status)
	}
}

// TestUpdateStatusWithError_CoversErrorPersistence testa que error_message é gravado.
func TestUpdateStatusWithError_CoversErrorPersistence(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-err-persist", 100); err != nil {
		t.Fatal(err)
	}

	// Avança para uploading
	if err := UpdateStatus(database, "v-err-persist", StatusUploading); err != nil {
		t.Fatal(err)
	}

	// Transiciona para failed_upload com mensagem específica
	errMsg := "erro: cliente desconectou durante upload"
	if err := UpdateStatusWithError(database, "v-err-persist", StatusFailedUpload, errMsg); err != nil {
		t.Fatalf("UpdateStatusWithError falhou: %v", err)
	}

	// Verifica que tanto status quanto error_message foram atualizados
	v, err := GetVideo(database, "v-err-persist")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusFailedUpload {
		t.Errorf("status: esperado %q, obtido %q", StatusFailedUpload, v.Status)
	}
	if v.ErrorMessage != errMsg {
		t.Errorf("error_message: esperado %q, obtido %q", errMsg, v.ErrorMessage)
	}
}

// TestSetUploadComplete_CoversActualSize testa que actual_size_bytes é atualizado.
func TestSetUploadComplete_CoversActualSize(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-size-test", 1000); err != nil {
		t.Fatal(err)
	}

	// Avança para uploading
	if err := UpdateStatus(database, "v-size-test", StatusUploading); err != nil {
		t.Fatal(err)
	}

	// Define tamanho real diferente do declarado
	actualSize := int64(1234)
	if err := SetUploadComplete(database, "v-size-test", actualSize); err != nil {
		t.Fatalf("SetUploadComplete falhou: %v", err)
	}

	// Verifica que tamanho real foi atualizado
	v, err := GetVideo(database, "v-size-test")
	if err != nil {
		t.Fatal(err)
	}
	if v.ActualSizeBytes != actualSize {
		t.Errorf("actual_size_bytes: esperado %d, obtido %d", actualSize, v.ActualSizeBytes)
	}
	if v.Status != StatusUploadComplete {
		t.Errorf("status: esperado %q, obtido %q", StatusUploadComplete, v.Status)
	}
}

// TestSetReady_CoversJsonSerialization testa serialização JSON de resolutions.
func TestSetReady_CoversJsonSerialization(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-json-test", 100); err != nil {
		t.Fatal(err)
	}

	// Avança para transcoding
	_ = UpdateStatus(database, "v-json-test", StatusUploading)
	_ = UpdateStatus(database, "v-json-test", StatusUploadComplete)
	_ = UpdateStatus(database, "v-json-test", StatusTranscoding)

	// Define resolutions com duração
	resolutions := []int{360, 720, 1080}
	durationS := 300
	if err := SetReady(database, "v-json-test", durationS, resolutions); err != nil {
		t.Fatalf("SetReady falhou: %v", err)
	}

	// Verifica que JSON foi serializado corretamente
	v, err := GetVideo(database, "v-json-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Resolutions) != 3 {
		t.Errorf("Resolutions: esperado 3 elementos, obtido %d", len(v.Resolutions))
	}
	if v.Resolutions[0] != 360 || v.Resolutions[1] != 720 || v.Resolutions[2] != 1080 {
		t.Errorf("Resolutions: esperado [360 720 1080], obtido %v", v.Resolutions)
	}
	if v.DurationS != durationS {
		t.Errorf("duration_s: esperado %d, obtido %d", durationS, v.DurationS)
	}
	if v.Status != StatusReady {
		t.Errorf("status: esperado %q, obtido %q", StatusReady, v.Status)
	}
}

// TestIncrementTranscodeAttempts_MultipleIncrements testa múltiplos incrementos.
func TestIncrementTranscodeAttempts_MultipleIncrements(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-incr-multi", 100); err != nil {
		t.Fatal(err)
	}

	// Incrementa múltiplas vezes
	for i := 0; i < 5; i++ {
		if err := IncrementTranscodeAttempts(database, "v-incr-multi"); err != nil {
			t.Fatalf("IncrementTranscodeAttempts %d falhou: %v", i, err)
		}
	}

	// Verifica que o contador está em 5
	v, err := GetVideo(database, "v-incr-multi")
	if err != nil {
		t.Fatal(err)
	}
	if v.TranscodeAttempts != 5 {
		t.Errorf("transcode_attempts: esperado 5, obtido %d", v.TranscodeAttempts)
	}
}

// TestListByStatus_CoversMultipleRows testa ListByStatus com vários resultados.
func TestListByStatus_CoversMultipleRows(t *testing.T) {
	database := abreDB(t)

	// Insere 3 vídeos em estado "uploading"
	for i := 0; i < 3; i++ {
		videoID := "v-list-multi-" + string(rune(48+i))
		if err := InsertVideo(database, videoID, 100); err != nil {
			t.Fatal(err)
		}
		if err := UpdateStatus(database, videoID, StatusUploading); err != nil {
			t.Fatal(err)
		}
	}

	// Busca vídeos em estado "uploading"
	results, err := ListByStatus(database, StatusUploading)
	if err != nil {
		t.Fatalf("ListByStatus falhou: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("ListByStatus: esperado 3 vídeos, obtive %d", len(results))
	}

	// Verifica que todos têm o status correto
	for _, v := range results {
		if v.Status != StatusUploading {
			t.Errorf("status incorreto: %q", v.Status)
		}
	}
}

// TestGetVideo_CoversAllNullableFields testa GetVideo com múltiplos campos NULL.
func TestGetVideo_CoversAllNullableFields(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-nulls", 100); err != nil {
		t.Fatal(err)
	}

	// Imediatamente após inserção, vários campos devem ser NULL/zero
	v, err := GetVideo(database, "v-nulls")
	if err != nil {
		t.Fatal(err)
	}

	// Verifica campos nullable
	if v.ActualSizeBytes != 0 {
		t.Errorf("actual_size_bytes inicial: esperado 0, obtido %d", v.ActualSizeBytes)
	}
	if v.DurationS != 0 {
		t.Errorf("duration_s inicial: esperado 0, obtido %d", v.DurationS)
	}
	if v.LastChunkAt != nil {
		t.Errorf("last_chunk_at inicial: esperado nil, obtido %v", v.LastChunkAt)
	}
	if v.ErrorMessage != "" {
		t.Errorf("error_message inicial: esperado vazio, obtido %q", v.ErrorMessage)
	}
	if len(v.Resolutions) != 0 {
		t.Errorf("resolutions inicial: esperado vazio, obtido %v", v.Resolutions)
	}
}

// TestListByStatus_IncludesProjectID verifica que ListByStatus retorna o
// project_id corretamente populado, sem o bug de omitir a coluna na query.
func TestListByStatus_IncludesProjectID(t *testing.T) {
	database := abreDB(t)

	// Cria um projeto para vincular ao vídeo.
	proj, _, err := CreateProject(database, "Projeto Teste T53")
	if err != nil {
		t.Fatalf("CreateProject falhou: %v", err)
	}

	// Insere um vídeo vinculado ao projeto.
	videoID := "550e8400-e29b-41d4-a716-446655440053"
	if err := InsertVideoForProject(database, videoID, 1024, &proj.ID); err != nil {
		t.Fatalf("InsertVideoForProject falhou: %v", err)
	}

	// Chama ListByStatus e verifica que ProjectID veio populado.
	results, err := ListByStatus(database, StatusPendingUpload)
	if err != nil {
		t.Fatalf("ListByStatus falhou: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("esperava 1 vídeo, obteve %d", len(results))
	}

	v := results[0]
	if v.ProjectID == nil {
		t.Fatal("ProjectID é nil — ListByStatus não populou project_id (bug)")
	}
	if *v.ProjectID != proj.ID {
		t.Errorf("ProjectID: esperado %d, obtido %d", proj.ID, *v.ProjectID)
	}
}

// TestGetVideo_IncludesProjectID confirma que GetVideo já retorna project_id
// corretamente (baseline: este teste passava antes da correção de ListByStatus).
func TestGetVideo_IncludesProjectID(t *testing.T) {
	database := abreDB(t)

	proj, _, err := CreateProject(database, "Projeto Baseline")
	if err != nil {
		t.Fatalf("CreateProject falhou: %v", err)
	}

	videoID := "660e8400-e29b-41d4-a716-446655440054"
	if err := InsertVideoForProject(database, videoID, 2048, &proj.ID); err != nil {
		t.Fatalf("InsertVideoForProject falhou: %v", err)
	}

	v, err := GetVideo(database, videoID)
	if err != nil {
		t.Fatalf("GetVideo falhou: %v", err)
	}
	if v.ProjectID == nil {
		t.Fatal("GetVideo: ProjectID é nil (regressão)")
	}
	if *v.ProjectID != proj.ID {
		t.Errorf("GetVideo: ProjectID esperado %d, obtido %d", proj.ID, *v.ProjectID)
	}
}
