package models

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Project representa um "projeto interno" (issue #6, T32): um namespace
// próprio — com diretório raiz isolado dentro de MEDIA_DIR e chave mestra
// própria — usado por um app/ambiente (produção, staging, teste, ...) que
// integra com o Streamedia.
//
// MasterKeyHash é o hash SHA-256 (hex) da chave mestra — a chave em texto
// puro nunca é persistida; é devolvida ao cliente uma única vez, no momento
// da criação (ver CreateProject).
type Project struct {
	ID            int64
	Name          string
	Slug          string
	RootDir       string
	MasterKeyHash string
	CreatedAt     time.Time
}

// HashMasterKey calcula o hash SHA-256 (hex) de uma chave mestra em texto
// puro — usado tanto para persistir quanto para validar uma chave recebida
// (compare o hash recebido com o armazenado).
func HashMasterKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// generateMasterKey gera uma chave mestra aleatória em texto puro, com
// entropia equivalente aos demais segredos do sistema (32 bytes, hex).
func generateMasterKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("erro ao gerar chave mestra: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Slugify normaliza um nome de projeto em um slug: minúsculas, sem
// acentos, espaços e separadores convertidos em "-", e apenas caracteres
// em [a-z0-9-] preservados. Múltiplos "-" consecutivos são colapsados e
// "-" nas extremidades são removidos.
//
// O resultado é usado tanto como identificador único quanto como nome do
// diretório raiz do projeto no disco — por isso precisa ser estável e
// seguro para uso em paths.
func Slugify(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(stripDiacritics(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// stripDiacritics remove acentos (á → a, ç → c, ã → a, ...) por
// decomposição Unicode + remoção de marcas de combinação — evita depender
// de uma tabela de transliteração mantida manualmente.
func stripDiacritics(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) { // marca de combinação (acento)
			continue
		}
		b.WriteRune(r)
	}
	return decompose(b.String())
}

// decompose normaliza runas acentuadas comuns em português para sua forma
// base + marca de combinação, para que stripDiacritics possa removê-la.
// Implementação simples por tabela — cobre o alfabeto latino estendido
// usado em nomes em português; não pretende ser uma normalização Unicode
// completa (NFD), que exigiria uma dependência externa.
func decompose(s string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "õ", "o", "ô", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n",
		"Á", "A", "À", "A", "Ã", "A", "Â", "A", "Ä", "A",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
		"Ó", "O", "Ò", "O", "Õ", "O", "Ô", "O", "Ö", "O",
		"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
		"Ç", "C", "Ñ", "N",
	)
	return replacer.Replace(s)
}

// uniqueSlug resolve colisões anexando "-2", "-3", ... ao slug base —
// conforme pedido na issue #6 ("se o projeto já existir [...] meta um -2
// ou -3 etc ao final da Key"). Verifica a existência consultando o banco
// diretamente; aceita uma colisão de corrida (ver nota em CreateProject).
func uniqueSlug(db *sql.DB, base string) (string, error) {
	candidate := base
	for n := 2; ; n++ {
		var exists int
		err := db.QueryRow(`SELECT 1 FROM projects WHERE slug = ?`, candidate).Scan(&exists)
		if err == sql.ErrNoRows {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("erro ao verificar unicidade do slug: %w", err)
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

// CreateProject cria um novo projeto: gera o slug (resolvendo colisões),
// define o diretório raiz como o próprio slug (relativo a MEDIA_DIR — quem
// monta o path final é o chamador, que conhece a config) e gera uma nova
// chave mestra aleatória.
//
// Retorna o Project persistido (com o hash da chave) e a chave mestra em
// texto puro — o ÚNICO momento em que ela existe fora do hash. O chamador
// deve devolvê-la ao usuário e nunca logá-la ou persisti-la em claro.
//
// Nota sobre corrida: a verificação de unicidade do slug (uniqueSlug) e o
// INSERT não são atômicos; em caso de corrida, a constraint UNIQUE(slug) no
// banco rejeita o INSERT duplicado e o erro é propagado — não há
// repetição automática, pois isso é considerado extremamente raro
// (criação de projetos é uma operação administrativa pouco frequente).
func CreateProject(db *sql.DB, name string) (*Project, string, error) {
	base := Slugify(name)
	if base == "" {
		return nil, "", fmt.Errorf("nome de projeto resulta em slug vazio: %q", name)
	}

	slug, err := uniqueSlug(db, base)
	if err != nil {
		return nil, "", err
	}

	masterKey, err := generateMasterKey()
	if err != nil {
		return nil, "", err
	}
	hash := HashMasterKey(masterKey)

	res, err := db.Exec(
		`INSERT INTO projects (name, slug, root_dir, master_key_hash) VALUES (?, ?, ?, ?)`,
		name, slug, slug, hash,
	)
	if err != nil {
		return nil, "", fmt.Errorf("erro ao inserir projeto: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, "", fmt.Errorf("erro ao obter id do projeto criado: %w", err)
	}

	project, err := GetProjectByID(db, id)
	if err != nil {
		return nil, "", err
	}
	return project, masterKey, nil
}

// scanProject lê uma linha da tabela projects para a struct Project,
// tratando o parsing de created_at (formato de datetime do SQLite).
func scanProject(scan func(dest ...any) error) (*Project, error) {
	var p Project
	var createdAt string
	if err := scan(&p.ID, &p.Name, &p.Slug, &p.RootDir, &p.MasterKeyHash, &createdAt); err != nil {
		return nil, err
	}
	p.CreatedAt = parseDateTime(createdAt)
	return &p, nil
}

const selectProjectColumns = `id, name, slug, root_dir, master_key_hash, created_at`

// GetProjectByID busca um projeto pelo seu ID. Retorna sql.ErrNoRows se não encontrado.
func GetProjectByID(db *sql.DB, id int64) (*Project, error) {
	row := db.QueryRow(`SELECT `+selectProjectColumns+` FROM projects WHERE id = ?`, id)
	return scanProject(row.Scan)
}

// GetProjectBySlug busca um projeto pelo seu slug. Retorna sql.ErrNoRows se não encontrado.
func GetProjectBySlug(db *sql.DB, slug string) (*Project, error) {
	row := db.QueryRow(`SELECT `+selectProjectColumns+` FROM projects WHERE slug = ?`, slug)
	return scanProject(row.Scan)
}

// GetProjectByMasterKeyHash busca o projeto cuja chave mestra tem o hash
// informado — usado para autenticar requisições que apresentam a chave
// mestra em texto puro (ex. header X-Project-Key, T33): o chamador calcula
// HashMasterKey(chave recebida) e resolve o projeto a partir do hash, sem
// nunca persistir ou comparar a chave em texto puro.
// Retorna sql.ErrNoRows se nenhum projeto corresponder.
func GetProjectByMasterKeyHash(db *sql.DB, hash string) (*Project, error) {
	row := db.QueryRow(`SELECT `+selectProjectColumns+` FROM projects WHERE master_key_hash = ?`, hash)
	return scanProject(row.Scan)
}

// ResolveVideoRootDir devolve o diretório raiz (relativo a MEDIA_DIR) onde
// os arquivos de um vídeo devem ser gravados/lidos — issue #6, T34: cada
// projeto isola seus vídeos sob <MEDIA_DIR>/<slug>/<video_id>/...
//
// projectID nil devolve "" (layout legado: <MEDIA_DIR>/<video_id>/...) —
// preserva compatibilidade para o raríssimo caso de um vídeo ainda não
// migrado (ver internal/jobs.MigrateLegacyVideos, que assume todos os
// vídeos antigos para um projeto "legacy" na inicialização).
func ResolveVideoRootDir(db *sql.DB, projectID *int64) (string, error) {
	if projectID == nil {
		return "", nil
	}
	project, err := GetProjectByID(db, *projectID)
	if err != nil {
		return "", err
	}
	return project.RootDir, nil
}

// ListProjects retorna todos os projetos cadastrados, ordenados pelo nome.
func ListProjects(db *sql.DB) ([]*Project, error) {
	rows, err := db.Query(`SELECT ` + selectProjectColumns + ` FROM projects ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar projetos: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p, err := scanProject(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler projeto: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
