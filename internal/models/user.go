package models

import (
	"database/sql"
	"fmt"
	"time"
)

// Roles de usuário e seus níveis numéricos (menor = mais poder).
const (
	RoleDev     = "dev"
	RoleAdmin   = "admin"
	RoleACL     = "acl"
	RoleManager = "manager"
)

// RoleLevelNum mapeia o nome da role para o level_num correspondente.
func RoleLevelNum(role string) (int, error) {
	switch role {
	case RoleDev:
		return 1, nil
	case RoleAdmin:
		return 2, nil
	case RoleACL:
		return 3, nil
	case RoleManager:
		return 4, nil
	default:
		return 0, fmt.Errorf("role desconhecida: %q", role)
	}
}

// AllRoles retorna todas as roles válidas no sistema.
func AllRoles() []string {
	return []string{RoleDev, RoleAdmin, RoleACL, RoleManager}
}

// User representa um usuário autenticado via Google OAuth.
type User struct {
	ID        int64
	Email     string
	Name      string
	Picture   string
	CreatedAt time.Time
}

// UserRole representa uma role atribuída a um usuário.
type UserRole struct {
	UserID    int64
	Role      string
	LevelNum  int
	GrantedBy int64 // 0 se concedida automaticamente (primeiro login)
	GrantedAt time.Time
}

// EffectiveLevel calcula o nível efetivo do usuário a partir das suas roles.
// É o menor level_num — quanto menor o número, maior o poder.
// Um usuário sem roles retorna 99 (nível mais baixo possível, sem poder algum).
//
// Exemplos:
//   - [admin=2, manager=4] → 2
//   - [acl=3, manager=4] → 3
//   - [manager=4] → 4
//   - [] → 99
func EffectiveLevel(roles []UserRole) int {
	if len(roles) == 0 {
		return 99
	}
	min := roles[0].LevelNum
	for _, r := range roles[1:] {
		if r.LevelNum < min {
			min = r.LevelNum
		}
	}
	return min
}

// CanGrantRole verifica se um usuário (com o nível efetivo calculado de suas
// roles) pode conceder a role alvo a outro usuário. A regra é:
//
//	effective_level(grantee) > target_role_level_num → NEGADO
//
// O grantee só pode conceder roles com level_num IGUAL ou MAIOR que seu
// próprio nível efetivo (número menor = mais poder).
func CanGrantRole(granteeEffectiveLevel int, targetRole string) bool {
	targetLevel, err := RoleLevelNum(targetRole)
	if err != nil {
		return false
	}
	// granteeEffectiveLevel menor que targetLevel → mais poderoso → pode conceder
	// granteeEffectiveLevel maior que targetLevel → menos poderoso → negado
	return granteeEffectiveLevel <= targetLevel
}

// scanUser lê uma linha da tabela users para a struct User.
func scanUser(scan func(dest ...any) error) (*User, error) {
	var u User
	var createdAt string
	if err := scan(&u.ID, &u.Email, &u.Name, &u.Picture, &createdAt); err != nil {
		return nil, err
	}
	u.CreatedAt = parseDateTime(createdAt)
	return &u, nil
}

// scanUserRole lê uma linha da tabela user_roles para a struct UserRole.
func scanUserRole(scan func(dest ...any) error) (*UserRole, error) {
	var r UserRole
	var grantedBy sql.NullInt64
	var grantedAt string
	if err := scan(&r.UserID, &r.Role, &r.LevelNum, &grantedBy, &grantedAt); err != nil {
		return nil, err
	}
	if grantedBy.Valid {
		r.GrantedBy = grantedBy.Int64
	}
	r.GrantedAt = parseDateTime(grantedAt)
	return &r, nil
}

// InsertUser insere um novo usuário. Retorna o ID gerado.
func InsertUser(db *sql.DB, email, name, picture string) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO users (email, name, picture) VALUES (?, ?, ?)`,
		email, name, picture,
	)
	if err != nil {
		return 0, fmt.Errorf("erro ao inserir usuário: %w", err)
	}
	return result.LastInsertId()
}

// GetUserByEmail busca um usuário pelo email. Retorna sql.ErrNoRows se não encontrado.
func GetUserByEmail(db *sql.DB, email string) (*User, error) {
	row := db.QueryRow(
		`SELECT id, email, name, picture, created_at FROM users WHERE email = ?`,
		email,
	)
	return scanUser(row.Scan)
}

// GetUserByID busca um usuário pelo ID.
func GetUserByID(db *sql.DB, id int64) (*User, error) {
	row := db.QueryRow(
		`SELECT id, email, name, picture, created_at FROM users WHERE id = ?`,
		id,
	)
	return scanUser(row.Scan)
}

// CountUsers retorna o número total de usuários na tabela.
func CountUsers(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// ListUsers retorna todos os usuários com suas roles, ordenados por data de criação.
func ListUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(`SELECT id, email, name, picture, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar usuários: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear usuário: %w", err)
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// DeleteUser remove um usuário e suas roles (ON DELETE CASCADE).
func DeleteUser(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// UpdateUser atualiza nome e foto do usuário.
func UpdateUser(db *sql.DB, id int64, name, picture string) error {
	_, err := db.Exec(`UPDATE users SET name = ?, picture = ? WHERE id = ?`, name, picture, id)
	return err
}

// InsertUserRole concede uma role a um usuário.
func InsertUserRole(db *sql.DB, userID int64, role string, grantedBy int64) error {
	levelNum, err := RoleLevelNum(role)
	if err != nil {
		return err
	}
	// grantedBy = 0 significa sem referência (primeiro login, bootstrap)
	var grantedByArg interface{}
	if grantedBy == 0 {
		grantedByArg = nil
	} else {
		grantedByArg = grantedBy
	}
	_, err = db.Exec(
		`INSERT OR REPLACE INTO user_roles (user_id, role, level_num, granted_by) VALUES (?, ?, ?, ?)`,
		userID, role, levelNum, grantedByArg,
	)
	return err
}

// GetUserRoles busca todas as roles de um usuário.
func GetUserRoles(db *sql.DB, userID int64) ([]UserRole, error) {
	rows, err := db.Query(
		`SELECT user_id, role, level_num, granted_by, granted_at FROM user_roles WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar roles do usuário: %w", err)
	}
	defer rows.Close()

	var roles []UserRole
	for rows.Next() {
		r, err := scanUserRole(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear role: %w", err)
		}
		roles = append(roles, *r)
	}
	return roles, rows.Err()
}

// DeleteUserRole remove uma role específica de um usuário.
func DeleteUserRole(db *sql.DB, userID int64, role string) error {
	_, err := db.Exec(`DELETE FROM user_roles WHERE user_id = ? AND role = ?`, userID, role)
	return err
}

// DeleteAllUserRoles remove todas as roles de um usuário.
func DeleteAllUserRoles(db *sql.DB, userID int64) error {
	_, err := db.Exec(`DELETE FROM user_roles WHERE user_id = ?`, userID)
	return err
}

// SetUserRoles substitui todas as roles de um usuário pela lista fornecida.
// Executado em uma transação: remove todas as roles existentes e insere as novas.
// O chamador (handler) deve validar a regra de nível (CanGrantRole) antes de chamar.
func SetUserRoles(db *sql.DB, userID int64, roles []string, grantedBy int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	if err := DeleteAllUserRoles(db, userID); err != nil {
		return fmt.Errorf("erro ao remover roles existentes: %w", err)
	}

	for _, role := range roles {
		if err := InsertUserRole(db, userID, role, grantedBy); err != nil {
			return fmt.Errorf("erro ao inserir role %q: %w", role, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao comitar transação: %w", err)
	}
	return nil
}

// ListUsersWithRoles retorna todos os usuários com suas roles populadas.
func ListUsersWithRoles(db *sql.DB) ([]User, map[int64][]UserRole, error) {
	users, err := ListUsers(db)
	if err != nil {
		return nil, nil, err
	}

	rolesMap := make(map[int64][]UserRole, len(users))
	for _, u := range users {
		roles, err := GetUserRoles(db, u.ID)
		if err != nil {
			return nil, nil, err
		}
		rolesMap[u.ID] = roles
	}

	return users, rolesMap, nil
}
