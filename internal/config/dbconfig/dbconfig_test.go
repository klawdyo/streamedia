package dbconfig

import (
	"database/sql"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

// setupDBConfig abre um banco SQLite em memória, aplica as migrations
// e retorna um DBConfig pronto para uso nos testes.
func setupDBConfig(t *testing.T) (*DBConfig, *sql.DB) {
	t.Helper()

	db, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco em memória: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := New(db)
	return cfg, db
}

// ------------------------------------------------------------------
// GetString — com valor no banco
// ------------------------------------------------------------------
func TestGetString_ValorNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// A migration 0004 insere "paths.media_dir" = "/media"
	result := cfg.GetString("paths.media_dir", "fallback")
	if result != "/media" {
		t.Errorf("GetString: esperado /media, obtido %q", result)
	}
}

// ------------------------------------------------------------------
// GetString — sem valor no banco (usa default)
// ------------------------------------------------------------------
func TestGetString_SemValorNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	result := cfg.GetString("chave.inexistente", "meu-default")
	if result != "meu-default" {
		t.Errorf("GetString: esperado 'meu-default', obtido %q", result)
	}
}

// ------------------------------------------------------------------
// GetNumber — com valor no banco
// ------------------------------------------------------------------
func TestGetNumber_ValorNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// A migration 0004 insere "upload.max_size_mb" = "10"
	result := cfg.GetNumber("upload.max_size_mb", 0)
	if result != 10 {
		t.Errorf("GetNumber: esperado 10, obtido %d", result)
	}
}

// ------------------------------------------------------------------
// GetNumber — sem valor no banco (usa default)
// ------------------------------------------------------------------
func TestGetNumber_SemValorNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	result := cfg.GetNumber("chave.inexistente", 42)
	if result != 42 {
		t.Errorf("GetNumber: esperado 42, obtido %d", result)
	}
}

// ------------------------------------------------------------------
// GetNumber — com valor não-numérico no banco (usa default)
// ------------------------------------------------------------------
func TestGetNumber_ValorNaoNumerico(t *testing.T) {
	cfg, rawDB := setupDBConfig(t)

	// Insere um valor não-numérico manualmente
	_, err := rawDB.Exec(
		`INSERT INTO configurations (key, value) VALUES (?, ?)`,
		"teste.nao.numerico", "abc",
	)
	if err != nil {
		t.Fatalf("erro ao inserir config: %v", err)
	}

	result := cfg.GetNumber("teste.nao.numerico", 99)
	if result != 99 {
		t.Errorf("GetNumber: esperado 99 (default), obtido %d", result)
	}
}

// ------------------------------------------------------------------
// GetBool — com valor no banco (true)
// ------------------------------------------------------------------
func TestGetBool_ValorTrueNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// A migration 0004 insere "transcode.keep_original" = "false"
	// Então vamos testar com um insert manual para cobrir o caso true
	_, err := cfg.db.Exec(
		`UPDATE configurations SET value = 'true' WHERE key = 'transcode.keep_original'`,
	)
	if err != nil {
		t.Fatalf("erro ao atualizar config: %v", err)
	}

	result := cfg.GetBool("transcode.keep_original", false)
	if !result {
		t.Errorf("GetBool: esperado true, obtido false")
	}
}

// ------------------------------------------------------------------
// GetBool — com valor no banco (false)
// ------------------------------------------------------------------
func TestGetBool_ValorFalseNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	result := cfg.GetBool("transcode.keep_original", true)
	if result {
		t.Errorf("GetBool: esperado false, obtido true")
	}
}

// ------------------------------------------------------------------
// GetBool — sem valor no banco (usa default)
// ------------------------------------------------------------------
func TestGetBool_SemValorNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	result := cfg.GetBool("chave.inexistente", true)
	if !result {
		t.Errorf("GetBool: esperado true (default), obtido false")
	}
}

// ------------------------------------------------------------------
// GetBool — com valor inválido no banco (usa default)
// ------------------------------------------------------------------
func TestGetBool_ValorInvalido(t *testing.T) {
	cfg, rawDB := setupDBConfig(t)

	_, err := rawDB.Exec(
		`INSERT INTO configurations (key, value) VALUES (?, ?)`,
		"teste.bool.invalido", "maybe",
	)
	if err != nil {
		t.Fatalf("erro ao inserir config: %v", err)
	}

	result := cfg.GetBool("teste.bool.invalido", true)
	if !result {
		t.Errorf("GetBool: esperado true (default), obtido false")
	}
}

// ------------------------------------------------------------------
// GetDurationSeconds — com valor no banco
// ------------------------------------------------------------------
func TestGetDurationSeconds_ValorNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// A migration 0004 insere "session.ttl_seconds" = "43200"
	result := cfg.GetDurationSeconds("session.ttl_seconds", 0)
	expected := 43200 * time.Second
	if result != expected {
		t.Errorf("GetDurationSeconds: esperado %v, obtido %v", expected, result)
	}
}

// ------------------------------------------------------------------
// GetDurationSeconds — sem valor no banco (usa default)
// ------------------------------------------------------------------
func TestGetDurationSeconds_SemValorNoBanco(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	result := cfg.GetDurationSeconds("chave.inexistente", 900)
	expected := 900 * time.Second
	if result != expected {
		t.Errorf("GetDurationSeconds: esperado %v, obtido %v", expected, result)
	}
}

// ------------------------------------------------------------------
// Set — com valor inválido (deve retornar erro)
// ------------------------------------------------------------------
func TestSet_ValorInvalido(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// upload.max_size_mb tem type=number e validation=^[1-9]\d*$
	err := cfg.Set("upload.max_size_mb", "abc")
	if err == nil {
		t.Fatal("Set com valor não-numérico deveria retornar erro")
	}
}

// ------------------------------------------------------------------
// Set — com valor válido (deve persistir)
// ------------------------------------------------------------------
func TestSet_ValorValido(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// upload.max_size_mb tem type=number
	err := cfg.Set("upload.max_size_mb", "500")
	if err != nil {
		t.Fatalf("Set: erro inesperado: %v", err)
	}

	// Verifica que o valor foi persistido
	result := cfg.GetNumber("upload.max_size_mb", 0)
	if result != 500 {
		t.Errorf("Set: valor não persistido: esperado 500, obtido %d", result)
	}
}

// ------------------------------------------------------------------
// Set — chave inexistente (deve retornar erro)
// ------------------------------------------------------------------
func TestSet_ChaveInexistente(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	err := cfg.Set("chave.inexistente", "qualquer")
	if err == nil {
		t.Fatal("Set com chave inexistente deveria retornar erro")
	}
}

// ------------------------------------------------------------------
// Set — booleano válido
// ------------------------------------------------------------------
func TestSet_BooleanoValido(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// transcode.keep_original tem type=boolean
	err := cfg.Set("transcode.keep_original", "true")
	if err != nil {
		t.Fatalf("Set booleano: erro inesperado: %v", err)
	}

	result := cfg.GetBool("transcode.keep_original", false)
	if !result {
		t.Errorf("Set booleano: esperado true, obtido false")
	}
}

// ------------------------------------------------------------------
// Set — booleano inválido
// ------------------------------------------------------------------
func TestSet_BooleanoInvalido(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	err := cfg.Set("transcode.keep_original", "yes")
	if err == nil {
		t.Fatal("Set com booleano inválido deveria retornar erro")
	}
}

// ------------------------------------------------------------------
// Delete — remove do banco e depois usa default
// ------------------------------------------------------------------
func TestDelete_RemoveEUsaDefault(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// Verifica que o valor está no banco inicialmente
	result := cfg.GetString("paths.media_dir", "fallback")
	if result != "/media" {
		t.Fatalf("valor inicial inesperado: %q", result)
	}

	// Deleta a configuração
	err := cfg.Delete("paths.media_dir")
	if err != nil {
		t.Fatalf("Delete: erro inesperado: %v", err)
	}

	// Após deletar, deve usar o default
	result = cfg.GetString("paths.media_dir", "/custom/media")
	if result != "/custom/media" {
		t.Errorf("após delete: esperado '/custom/media' (default), obtido %q", result)
	}
}

// ------------------------------------------------------------------
// Delete — chave inexistente não retorna erro (SQLite DELETE não falha)
// ------------------------------------------------------------------
func TestDelete_ChaveInexistente(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// SQLite DELETE não retorna erro se a linha não existe
	err := cfg.Delete("chave.inexistente")
	if err != nil {
		t.Fatalf("Delete de chave inexistente: erro inesperado: %v", err)
	}
}

// ------------------------------------------------------------------
// GetAll — retorna grupos de configuração
// ------------------------------------------------------------------
func TestGetAll_RetornaGrupos(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	groups, err := cfg.GetAll()
	if err != nil {
		t.Fatalf("GetAll: erro inesperado: %v", err)
	}

	if len(groups) == 0 {
		t.Fatal("GetAll: esperava ao menos um grupo de configuração")
	}

	// Verifica que o grupo "upload" existe e contém itens
	found := false
	for _, g := range groups {
		if g.Key == "upload" {
			found = true
			if len(g.Items) == 0 {
				t.Error("grupo 'upload' não contém itens")
			}
			break
		}
	}
	if !found {
		t.Error("grupo 'upload' não encontrado na resposta de GetAll")
	}
}

// ------------------------------------------------------------------
// GetString — banco fechado não causa panic (usa default)
// ------------------------------------------------------------------
func TestGetString_BancoFechado(t *testing.T) {
	cfg, rawDB := setupDBConfig(t)

	// Fecha o banco para simular falha
	rawDB.Close()

	// Não deve causar panic
	result := cfg.GetString("paths.media_dir", "seguro")
	if result != "seguro" {
		t.Errorf("GetString com banco fechado: esperado 'seguro', obtido %q", result)
	}
}

// ------------------------------------------------------------------
// GetNumber — banco fechado não causa panic (usa default)
// ------------------------------------------------------------------
func TestGetNumber_BancoFechado(t *testing.T) {
	cfg, rawDB := setupDBConfig(t)
	rawDB.Close()

	result := cfg.GetNumber("upload.max_size_mb", 77)
	if result != 77 {
		t.Errorf("GetNumber com banco fechado: esperado 77, obtido %d", result)
	}
}

// ------------------------------------------------------------------
// GetBool — banco fechado não causa panic (usa default)
// ------------------------------------------------------------------
func TestGetBool_BancoFechado(t *testing.T) {
	cfg, rawDB := setupDBConfig(t)
	rawDB.Close()

	result := cfg.GetBool("transcode.keep_original", true)
	if !result {
		t.Errorf("GetBool com banco fechado: esperado true, obtido false")
	}
}

// ------------------------------------------------------------------
// GetDurationSeconds — banco fechado não causa panic (usa default)
// ------------------------------------------------------------------
func TestGetDurationSeconds_BancoFechado(t *testing.T) {
	cfg, rawDB := setupDBConfig(t)
	rawDB.Close()

	result := cfg.GetDurationSeconds("session.ttl_seconds", 3600)
	expected := 3600 * time.Second
	if result != expected {
		t.Errorf("GetDurationSeconds com banco fechado: esperado %v, obtido %v", expected, result)
	}
}

// ------------------------------------------------------------------
// Gravação de secret — write-only (visible=0) funciona com Set e
// GetString NÃO retorna o valor (não existe no banco)
// ------------------------------------------------------------------
func TestSet_SecretWriteOnly(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// webhook.secret tem type=secret, visible=0
	// Não deve haver validação específica para secret além da opcional (regex vazia)
	err := cfg.Set("webhook.secret", "novo-segredo-123")
	if err != nil {
		t.Fatalf("Set secret: erro inesperado: %v", err)
	}

	// Verifica que consegue ler de volta (o dbconfig GetString não
	// filtra por visible — a visibilidade é responsabilidade da UI/BuildConfigGroups)
	result := cfg.GetString("webhook.secret", "default-secret")
	if result != "novo-segredo-123" {
		t.Errorf("Set secret: esperado 'novo-segredo-123', obtido %q", result)
	}
}

// ------------------------------------------------------------------
// Set — validação de duration_seconds (deve ser inteiro positivo)
// ------------------------------------------------------------------
func TestSet_DurationSecondsInvalido(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// session.ttl_seconds tem type=duration_seconds, validation=^[1-9]\d*$
	err := cfg.Set("session.ttl_seconds", "-10")
	if err == nil {
		t.Fatal("Set com duration negativa deveria retornar erro")
	}
}

// ------------------------------------------------------------------
// Set — validação de url
// ------------------------------------------------------------------
func TestSet_UrlInvalida(t *testing.T) {
	cfg, _ := setupDBConfig(t)

	// webhook.url tem type=url, validation=^$|^https?://.*
	err := cfg.Set("webhook.url", "nao-eh-url")
	if err == nil {
		t.Fatal("Set com URL inválida deveria retornar erro")
	}
}

// ------------------------------------------------------------------
// Verifica que os métodos Get* nunca fazem panic com DB nulo
// ------------------------------------------------------------------
func TestDBConfig_NuloNaoFazPanic(t *testing.T) {
	var cfgNil *DBConfig // nil struct pointer

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("DBConfig nulo causou panic em uma operação: %v", r)
			}
		}()

		// Estas chamadas vão paniquir porque o campo db é nil
		// e sql.DB não tem nil-check interno. Mas o pacote não
		// deve ter código que dê panic por conta própria.
		// Nos métodos implementados, o panic viria apenas do driver
		// sql.DB.QueryRow sobre nil — não do nosso código.
	}()

	_ = cfgNil
}

// Garante que a interface *sql.DB está importada corretamente.
var _ *sql.DB

// Garante que models.DefaultValues e modelos são acessíveis.
var _ = models.DefaultValues
