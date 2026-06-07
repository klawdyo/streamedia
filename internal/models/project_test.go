package models

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/klawdyo/streamedia/internal/db"
)

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Trip Produção", "trip-producao"},
		{"  Multi   Espaços!! ", "multi-espacos"},
		{"API de Teste #2", "api-de-teste-2"},
		{"São Paulo", "sao-paulo"},
		{"---já-com-traços---", "ja-com-tracos"},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, esperado %q", c.in, got, c.want)
		}
	}
}

func TestCreateProject_PersistsAndGeneratesMasterKey(t *testing.T) {
	conn := abreDBProject(t)

	project, masterKey, err := CreateProject(conn, "Trip Produção")
	if err != nil {
		t.Fatalf("CreateProject retornou erro: %v", err)
	}

	if project.Slug != "trip-producao" {
		t.Errorf("Slug: esperado %q, obtido %q", "trip-producao", project.Slug)
	}
	if project.RootDir != "trip-producao" {
		t.Errorf("RootDir: esperado %q, obtido %q", "trip-producao", project.RootDir)
	}
	if masterKey == "" {
		t.Fatal("master key em texto puro não deveria ser vazia")
	}
	if project.MasterKeyHash != HashMasterKey(masterKey) {
		t.Error("MasterKeyHash não corresponde ao hash da chave devolvida")
	}
	if project.MasterKeyHash == masterKey {
		t.Error("MasterKeyHash não deveria ser igual à chave em texto puro")
	}
}

func TestCreateProject_ResolvesSlugCollision(t *testing.T) {
	conn := abreDBProject(t)

	p1, _, err := CreateProject(conn, "Trip")
	if err != nil {
		t.Fatalf("CreateProject (1ª) retornou erro: %v", err)
	}
	p2, _, err := CreateProject(conn, "Trip")
	if err != nil {
		t.Fatalf("CreateProject (2ª) retornou erro: %v", err)
	}
	p3, _, err := CreateProject(conn, "Trip")
	if err != nil {
		t.Fatalf("CreateProject (3ª) retornou erro: %v", err)
	}

	if p1.Slug != "trip" {
		t.Errorf("1º slug: esperado %q, obtido %q", "trip", p1.Slug)
	}
	if p2.Slug != "trip-2" {
		t.Errorf("2º slug: esperado %q, obtido %q", "trip-2", p2.Slug)
	}
	if p3.Slug != "trip-3" {
		t.Errorf("3º slug: esperado %q, obtido %q", "trip-3", p3.Slug)
	}
}

func TestGetProjectBySlug_NotFound(t *testing.T) {
	conn := abreDBProject(t)

	_, err := GetProjectBySlug(conn, "inexistente")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("esperado sql.ErrNoRows, obtido %v", err)
	}
}

func TestGetProjectByID_NotFound(t *testing.T) {
	conn := abreDBProject(t)

	_, err := GetProjectByID(conn, 999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("esperado sql.ErrNoRows, obtido %v", err)
	}
}

func TestListProjects_ReturnsAllOrderedByName(t *testing.T) {
	conn := abreDBProject(t)

	if _, _, err := CreateProject(conn, "Zeta"); err != nil {
		t.Fatalf("CreateProject(Zeta): %v", err)
	}
	if _, _, err := CreateProject(conn, "Alpha"); err != nil {
		t.Fatalf("CreateProject(Alpha): %v", err)
	}

	projects, err := ListProjects(conn)
	if err != nil {
		t.Fatalf("ListProjects retornou erro: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("esperava 2 projetos, obteve %d", len(projects))
	}
	if projects[0].Name != "Alpha" || projects[1].Name != "Zeta" {
		t.Errorf("ordem inesperada: %q, %q", projects[0].Name, projects[1].Name)
	}
}

func TestHashMasterKey_IsDeterministicAndDistinguishesInputs(t *testing.T) {
	h1 := HashMasterKey("abc")
	h2 := HashMasterKey("abc")
	h3 := HashMasterKey("xyz")

	if h1 != h2 {
		t.Error("HashMasterKey deveria ser determinístico para a mesma entrada")
	}
	if h1 == h3 {
		t.Error("HashMasterKey deveria gerar hashes diferentes para entradas diferentes")
	}
}

// abreDBProject abre um banco SQLite em memória com o schema aplicado, para
// isolar os testes de modelos sem depender de um arquivo no disco.
func abreDBProject(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("erro ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
