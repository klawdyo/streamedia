// Pacote google implementa os handlers de autenticação via Google OAuth 2.0.
//
// Fluxo OAuth:
//  1. GET /api/auth/google → HandleLogin: constrói URL OAuth do Google,
//     armazena state em cookie anti-CSRF e redireciona o navegador para
//     accounts.google.com.
//  2. O usuário autoriza no Google e é redirecionado de volta para
//     GET /api/auth/google/callback → HandleCallback.
//  3. HandleCallback valida o state anti-CSRF, troca o code por um token
//     de acesso, busca os dados do usuário (userinfo), cria ou atualiza
//     o registro local e emite um cookie de sessão (streamedia_session).
//  4. GET /api/auth/me → HandleMe: retorna os dados do usuário logado
//     (lidos do cookie de sessão via contexto injetado pelo middleware
//     RootAuth).
//
// Bootstrap: o primeiro usuário a fazer login (tabela users vazia) recebe
// automaticamente a role "dev" (nível máximo). Logins subsequentes só são
// permitidos para emails já cadastrados na tabela users.
//
// Todas as chamadas HTTP para o Google usam net/http padrão — sem
// bibliotecas externas de OAuth.
package google

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// Constantes do fluxo OAuth com Google.
const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

	// stateCookieName é o nome do cookie que armazena o parâmetro state
	// durante o fluxo OAuth para prevenção de CSRF.
	stateCookieName = "google_oauth_state"

	// stateTTL é a validade do cookie de state (5 minutos).
	stateTTL = 5 * time.Minute

	// SessionCookieName é o nome do cookie de sessão de navegador.
	// Compartilhado com o pacote admin, que o define como "streamedia_session".
	SessionCookieName = "streamedia_session"
)

// GoogleHandler agrupa as dependências dos handlers de autenticação Google.
// Os campos httpClient, tokenURL e userInfoURL são injetáveis para facilitar
// testes — em produção usa-se os defaults (http.DefaultClient e as URLs
// oficiais do Google).
type GoogleHandler struct {
	cfg         *config.Config
	db          *sql.DB
	httpClient  *http.Client
	tokenURL    string
	userInfoURL string
}

// NewGoogleHandler cria um GoogleHandler com as dependências injetadas.
// Usa http.DefaultClient e as URLs oficiais do Google como padrão.
func NewGoogleHandler(cfg *config.Config, db *sql.DB) *GoogleHandler {
	return &GoogleHandler{
		cfg:         cfg,
		db:          db,
		httpClient:  http.DefaultClient,
		tokenURL:    googleTokenURL,
		userInfoURL: googleUserInfoURL,
	}
}

// httpClientOrDefault retorna o http.Client configurado ou o default.
func (h *GoogleHandler) getHTTPClient() *http.Client {
	if h.httpClient != nil {
		return h.httpClient
	}
	return http.DefaultClient
}

// getTokenURL retorna a URL do endpoint de token configurada ou a padrão.
func (h *GoogleHandler) getTokenURL() string {
	if h.tokenURL != "" {
		return h.tokenURL
	}
	return googleTokenURL
}

// getUserInfoURL retorna a URL do endpoint userinfo configurada ou a padrão.
func (h *GoogleHandler) getUserInfoURL() string {
	if h.userInfoURL != "" {
		return h.userInfoURL
	}
	return googleUserInfoURL
}

// HandleLogin constrói a URL de autorização OAuth 2.0 do Google e redireciona
// o navegador para accounts.google.com.
//
// Parâmetros da URL:
//   - client_id: GOOGLE_CLIENT_ID
//   - redirect_uri: GOOGLE_REDIRECT_URL
//   - response_type: code
//   - scope: openid profile email
//   - state: string aleatória (32 bytes hex) armazenada em cookie para
//     validação anti-CSRF no callback.
//
// O cookie de state tem validade de 5 minutos e é HttpOnly, Secure
// (conforme cfg.SessionCookieSecure) e SameSite=Lax (necessário para
// que o navegador o envie no redirecionamento de volta do Google).
func (h *GoogleHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Gera state aleatório para prevenção de CSRF.
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao gerar state OAuth.")
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Armazena o state em cookie de curta duração.
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		Expires:  time.Now().Add(stateTTL),
		MaxAge:   int(stateTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Monta a URL de autorização do Google.
	authURL, _ := url.Parse(googleAuthURL)
	q := authURL.Query()
	q.Set("client_id", h.cfg.GoogleClientID)
	q.Set("redirect_uri", h.cfg.GoogleRedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	authURL.RawQuery = q.Encode()

	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

// HandleCallback processa o retorno do Google OAuth após o usuário autorizar
// o acesso. Fluxo:
//
//  1. Valida o state do cookie contra o query param (anti-CSRF).
//  2. Troca o authorization code por um token de acesso (POST para
//     https://oauth2.googleapis.com/token).
//  3. Busca os dados do usuário (GET para
//     https://www.googleapis.com/oauth2/v2/userinfo).
//  4. Verifica se a tabela users está vazia:
//     → vazia: bootstrap — cria o usuário e concede role "dev".
//     → não vazia: busca o email em users; se não existe → 403.
//     → existe: atualiza name e picture do usuário.
//  5. Emite cookie de sessão com user_id, roles e HMAC e redireciona para
//     /app.
func (h *GoogleHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// 1. Validação do state anti-CSRF.
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Cookie de estado OAuth ausente ou expirado.")
		return
	}
	queryState := r.URL.Query().Get("state")
	if queryState == "" || !auth.SecureCompare(stateCookie.Value, queryState) {
		apiresponse.Error(w, http.StatusBadRequest, "Parâmetro state inválido — possível ataque CSRF.")
		return
	}

	// Remove o cookie de state (já foi consumido).
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// 2. Troca code por token.
	code := r.URL.Query().Get("code")
	if code == "" {
		apiresponse.Error(w, http.StatusBadRequest, "Parâmetro code ausente na resposta do Google.")
		return
	}

	accessToken, err := h.exchangeCodeForToken(code)
	if err != nil {
		apiresponse.Error(w, http.StatusBadGateway, "Falha ao trocar código por token com o Google.")
		return
	}

	// 3. Busca dados do usuário no Google.
	googleUser, err := h.fetchGoogleUserInfo(accessToken)
	if err != nil {
		apiresponse.Error(w, http.StatusBadGateway, "Falha ao obter dados do usuário do Google.")
		return
	}

	// 4. Verifica se é o primeiro login (bootstrap) ou login de usuário existente.
	count, err := models.CountUsers(h.db)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao consultar usuários.")
		return
	}

	var user *models.User
	var roles []models.UserRole

	if count == 0 {
		// Bootstrap: primeiro usuário do sistema — cria com role "dev".
		userID, err := models.InsertUser(h.db, googleUser.Email, googleUser.Name, googleUser.Picture)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao criar usuário bootstrap.")
			return
		}
		// Concede role "dev" (grantedBy=0 = automático).
		if err := models.InsertUserRole(h.db, userID, models.RoleDev, 0); err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao conceder role de bootstrap.")
			return
		}
		user = &models.User{
			ID:      userID,
			Email:   googleUser.Email,
			Name:    googleUser.Name,
			Picture: googleUser.Picture,
		}
		roles = []models.UserRole{
			{UserID: userID, Role: models.RoleDev, LevelNum: 1},
		}
	} else {
		// Usuários já existem: busca pelo email.
		existingUser, err := models.GetUserByEmail(h.db, googleUser.Email)
		if err == sql.ErrNoRows {
			apiresponse.Error(w, http.StatusForbidden, "Email não autorizado.")
			return
		}
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar usuário.")
			return
		}

		// Atualiza nome e foto (podem ter mudado no Google).
		if err := models.UpdateUser(h.db, existingUser.ID, googleUser.Name, googleUser.Picture); err != nil {
			// Falha não crítica — loga e continua.
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao atualizar dados do usuário.")
			return
		}
		user = existingUser
		user.Name = googleUser.Name
		user.Picture = googleUser.Picture

		// Busca roles atuais do usuário.
		roles, err = models.GetUserRoles(h.db, user.ID)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar roles do usuário.")
			return
		}
	}

	// 5. Gera cookie de sessão com user_id e roles.
	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Role
	}

	sessionValue, expiresAt := auth.IssueSessionTokenWithUser(
		h.cfg.RootToken, user.ID, roleNames, h.cfg.SessionTTL,
	)

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionValue,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(h.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})

	// Redireciona para a aplicação.
	http.Redirect(w, r, "/app", http.StatusFound)
}

// HandleMe retorna os dados do usuário autenticado (email, nome, foto, roles
// e nível efetivo). O user_id é lido do contexto da requisição, injetado pelo
// middleware RootAuth a partir do cookie de sessão.
//
// Resposta JSON:
//
//	{
//	  "email": "...",
//	  "name": "...",
//	  "picture": "...",
//	  "roles": [{"role": "dev", "level_num": 1}],
//	  "effective_level": 1
//	}
func (h *GoogleHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		apiresponse.Error(w, http.StatusUnauthorized, "Usuário não autenticado.")
		return
	}

	user, err := models.GetUserByID(h.db, userID)
	if err == sql.ErrNoRows {
		apiresponse.Error(w, http.StatusNotFound, "Usuário não encontrado.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar usuário.")
		return
	}

	roles, err := models.GetUserRoles(h.db, userID)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao buscar roles do usuário.")
		return
	}

	type roleItem struct {
		Role     string `json:"role"`
		LevelNum int    `json:"level_num"`
	}

	roleItems := make([]roleItem, len(roles))
	for i, r := range roles {
		roleItems[i] = roleItem{Role: r.Role, LevelNum: r.LevelNum}
	}

	type meResponse struct {
		Email          string     `json:"email"`
		Name           string     `json:"name"`
		Picture        string     `json:"picture"`
		Roles          []roleItem `json:"roles"`
		EffectiveLevel int        `json:"effective_level"`
	}

	apiresponse.Success(w, http.StatusOK, meResponse{
		Email:          user.Email,
		Name:           user.Name,
		Picture:        user.Picture,
		Roles:          roleItems,
		EffectiveLevel: models.EffectiveLevel(roles),
	})
}

// --- Métodos auxiliares de comunicação com o Google (net/http puro) ---

// googleTokenResponse é a resposta JSON do endpoint de token do Google.
type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error,omitempty"`
}

// googleUserInfo é a resposta JSON do endpoint userinfo do Google.
type googleUserInfo struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// exchangeCodeForToken troca o authorization code por um token de acesso
// junto ao Google (POST no token endpoint configurado).
func (h *GoogleHandler) exchangeCodeForToken(code string) (string, error) {
	data := url.Values{}
	data.Set("client_id", h.cfg.GoogleClientID)
	data.Set("client_secret", h.cfg.GoogleClientSecret)
	data.Set("redirect_uri", h.cfg.GoogleRedirectURL)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")

	resp, err := h.getHTTPClient().Post(h.getTokenURL(), "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("erro ao chamar token endpoint do Google: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta do token endpoint: %w", err)
	}

	var tokenResp googleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("erro ao decodificar resposta do token endpoint: %w", err)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("erro do Google na troca de token: %s", tokenResp.Error)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token endpoint do Google retornou access_token vazio")
	}

	return tokenResp.AccessToken, nil
}

// fetchGoogleUserInfo busca os dados do usuário autenticado no Google
// (GET no userinfo endpoint configurado).
func (h *GoogleHandler) fetchGoogleUserInfo(accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequest(http.MethodGet, h.getUserInfoURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição userinfo: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := h.getHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao chamar userinfo endpoint do Google: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta do userinfo endpoint: %w", err)
	}

	var user googleUserInfo
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta do userinfo endpoint: %w", err)
	}

	if user.Email == "" {
		return nil, fmt.Errorf("userinfo endpoint retornou email vazio")
	}

	return &user, nil
}
