// Handlers de CRUD de usuários para o painel de administração.
// Todas as rotas são protegidas pelo middleware RootAuth.
package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/models"
)

// userResponse é a estrutura de resposta JSON para um usuário com suas roles.
type userResponse struct {
	ID             int64      `json:"id"`
	Email          string     `json:"email"`
	Name           string     `json:"name"`
	Picture        string     `json:"picture"`
	CreatedAt      time.Time  `json:"created_at"`
	Roles          []roleItem `json:"roles"`
	EffectiveLevel int        `json:"effective_level"`
}

// roleItem representa uma role na resposta JSON.
type roleItem struct {
	Role     string `json:"role"`
	LevelNum int    `json:"level_num"`
}

// createUserBody é o corpo esperado em POST /admin/users.
type createUserBody struct {
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Picture string   `json:"picture"`
	Roles   []string `json:"roles"`
}

// updateRolesBody é o corpo esperado em PUT /admin/users/{userID}/roles.
type updateRolesBody struct {
	Roles []string `json:"roles"`
}

// usersListResponse é o envelope da lista de usuários com total.
type usersListResponse struct {
	Users []userResponse `json:"users"`
	Total int            `json:"total"`
}

// HandleListUsers lista todos os usuários com suas roles.
// GET /admin/users
//
// Retorna JSON no formato:
//
//	{ users: [{id, email, name, picture, created_at, roles: [{role, level_num}]}], total }
func (h *AdminHandler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	users, rolesMap, err := models.ListUsersWithRoles(h.db)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao listar usuários.")
		return
	}

	// Garante slice vazio no JSON em vez de null
	if users == nil {
		users = []models.User{}
	}

	result := make([]userResponse, 0, len(users))
	for _, u := range users {
		ur := userResponse{
			ID:        u.ID,
			Email:     u.Email,
			Name:      u.Name,
			Picture:   u.Picture,
			CreatedAt: u.CreatedAt,
			Roles:     make([]roleItem, 0),
		}
		if roles, ok := rolesMap[u.ID]; ok {
			for _, r := range roles {
				ur.Roles = append(ur.Roles, roleItem{
					Role:     r.Role,
					LevelNum: r.LevelNum,
				})
			}
			ur.EffectiveLevel = models.EffectiveLevel(roles)
		}
		result = append(result, ur)
	}

	apiresponse.Success(w, http.StatusOK, usersListResponse{
		Users: result,
		Total: len(result),
	})
}

// HandleCreateUser cria um novo usuário e opcionalmente concede roles.
// POST /admin/users
//
// Body:
//
//	{
//	  "email": "usuario@exemplo.com",   // obrigatório
//	  "name": "Nome do Usuário",        // opcional — será atualizado no login
//	  "picture": "https://...",         // opcional — será atualizado no login
//	  "roles": ["admin", "manager"]     // opcional
//	}
//
// Se roles for informado, valida que o grantee tem permissão para conceder
// cada role. Se não, cria o usuário sem roles.
func (h *AdminHandler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body createUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}

	// Validação: email é obrigatório
	if body.Email == "" {
		apiresponse.Error(w, http.StatusBadRequest, "O campo email é obrigatório.")
		return
	}

	// Verifica se o email já existe antes de tentar inserir
	existing, err := models.GetUserByEmail(h.db, body.Email)
	if err != nil && err != sql.ErrNoRows {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao verificar email.")
		return
	}
	if existing != nil {
		apiresponse.Error(w, http.StatusConflict, "Já existe um usuário com este email.")
		return
	}

	// Se houver roles no body, valida a regra de nível para cada uma
	if len(body.Roles) > 0 {
		if err := h.validateRoleGrants(r, body.Roles); err != nil {
			apiresponse.Error(w, http.StatusForbidden, err.Error())
			return
		}
	}

	// Cria o usuário
	userID, err := models.InsertUser(h.db, body.Email, body.Name, body.Picture)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, fmt.Sprintf("Erro ao criar usuário: %v", err))
		return
	}

	// Concede as roles, se houver
	granteeUserID, _ := auth.GetUserIDFromContext(r.Context())
	for _, role := range body.Roles {
		if err := models.InsertUserRole(h.db, userID, role, granteeUserID); err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, fmt.Sprintf("Erro ao conceder role %s ao usuário: %v", role, err))
			return
		}
	}

	// Busca o usuário criado para retornar os dados completos
	user, err := models.GetUserByID(h.db, userID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar usuário criado.")
		return
	}

	// Busca as roles concedidas
	roles, err := models.GetUserRoles(h.db, userID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar roles do usuário.")
		return
	}

	apiresponse.Success(w, http.StatusCreated, h.buildUserResponse(user, roles))
}

// HandleUpdateUserRoles substitui todas as roles de um usuário pela lista fornecida.
// PUT /admin/users/{userID}/roles
//
// URL param: userID (chi)
// Body: { roles: ["admin", "manager"] }
//
// Valida que cada role é uma role reconhecida e que o grantee tem permissão
// para concedê-la. Usa models.SetUserRoles para a operação transacional.
func (h *AdminHandler) HandleUpdateUserRoles(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "ID de usuário inválido.")
		return
	}

	var body updateRolesBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Corpo da requisição inválido.")
		return
	}

	// Valida que cada role é uma role reconhecida pelo sistema
	for _, role := range body.Roles {
		if _, err := models.RoleLevelNum(role); err != nil {
			apiresponse.Error(w, http.StatusBadRequest, fmt.Sprint("Role inválida: ", role))
			return
		}
	}

	// Valida a regra de nível para cada role
	if err := h.validateRoleGrants(r, body.Roles); err != nil {
		apiresponse.Error(w, http.StatusForbidden, err.Error())
		return
	}

	// Obtém o grantee (quem está concedendo as roles)
	granteeUserID, _ := auth.GetUserIDFromContext(r.Context())

	// Substitui todas as roles do usuário
	if err := models.SetUserRoles(h.db, userID, body.Roles, granteeUserID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao atualizar roles do usuário.")
		return
	}

	// Busca o usuário atualizado para retornar
	user, err := models.GetUserByID(h.db, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			apiresponse.Error(w, http.StatusNotFound, "Usuário não encontrado.")
			return
		}
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar usuário.")
		return
	}

	// Busca as roles atualizadas
	roles, err := models.GetUserRoles(h.db, userID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar roles do usuário.")
		return
	}

	apiresponse.Success(w, http.StatusOK, h.buildUserResponse(user, roles))
}

// HandleDeleteUser remove um usuário do sistema.
// DELETE /admin/users/{userID}
//
// URL param: userID (chi)
//
// Impede que o usuário autenticado delete a si mesmo.
// Retorna 204 No Content em sucesso.
func (h *AdminHandler) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "ID de usuário inválido.")
		return
	}

	// Impede que o usuário delete a si mesmo
	if granteeUserID, ok := auth.GetUserIDFromContext(r.Context()); ok && granteeUserID == userID {
		apiresponse.Error(w, http.StatusForbidden, "Você não pode deletar seu próprio usuário.")
		return
	}

	// Verifica se o usuário existe antes de deletar
	_, err = models.GetUserByID(h.db, userID)
	if err == sql.ErrNoRows {
		apiresponse.Error(w, http.StatusNotFound, "Usuário não encontrado.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar usuário.")
		return
	}

	// Deleta o usuário (ON DELETE CASCADE remove as roles também)
	if err := models.DeleteUser(h.db, userID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao deletar usuário.")
		return
	}

	apiresponse.Success(w, http.StatusNoContent, nil)
}

// validateRoleGrants verifica se o grantee (usuário autenticado no contexto
// da requisição) tem permissão para conceder cada uma das roles solicitadas.
//
// Se não houver usuário no contexto — autenticação via Bearer/ROOT_TOKEN —,
// o acesso é total (sem restrição de nível).
//
// Retorna nil se todas as roles puderem ser concedidas, ou um erro com a
// mensagem apropriada em português.
func (h *AdminHandler) validateRoleGrants(r *http.Request, roles []string) error {
	granteeUserID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		// Autenticação via Bearer (ROOT_TOKEN): acesso total, sem restrição de nível.
		return nil
	}

	// Busca as roles do grantee para calcular o nível efetivo
	granteeRoles, err := models.GetUserRoles(h.db, granteeUserID)
	if err != nil {
		return fmt.Errorf("erro ao buscar roles do usuário autenticado: %w", err)
	}

	effectiveLevel := models.EffectiveLevel(granteeRoles)

	// Para cada role solicitada, verifica se o grantee pode concedê-la
	for _, role := range roles {
		if !models.CanGrantRole(effectiveLevel, role) {
			return fmt.Errorf("Você não tem permissão para conceder a role %s", role)
		}
	}

	return nil
}

// buildUserResponse monta a estrutura userResponse a partir de um models.User
// e sua lista de roles.
func (h *AdminHandler) buildUserResponse(user *models.User, roles []models.UserRole) userResponse {
	ur := userResponse{
		ID:             user.ID,
		Email:          user.Email,
		Name:           user.Name,
		Picture:        user.Picture,
		CreatedAt:      user.CreatedAt,
		Roles:          make([]roleItem, 0, len(roles)),
		EffectiveLevel: models.EffectiveLevel(roles),
	}
	for _, r := range roles {
		ur.Roles = append(ur.Roles, roleItem{
			Role:     r.Role,
			LevelNum: r.LevelNum,
		})
	}
	return ur
}
