package admin

import (
	"net/http"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
)

// SessionCookieName é o nome do cookie de sessão de navegador, emitido por
// HandleSessionLogin e validado por RootAuth.
const SessionCookieName = "streamedia_session"

// CSRFHeaderName é o header exigido por RootAuth em métodos não seguros
// (POST/PUT/PATCH/DELETE) quando a autenticação vem do cookie de sessão. Só
// pode ser definido por JavaScript same-origin (fetch), o que impede que uma
// requisição cross-site forjada o inclua.
const CSRFHeaderName = "X-Streamedia-Csrf"

// csrfHeaderValue é o valor esperado do header CSRFHeaderName.
const csrfHeaderValue = "1"

// sessionResponse é o corpo de resposta de login/logout de sessão.
type sessionResponse struct {
	Active    bool   `json:"active"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// HandleSessionLogin troca um Authorization: Bearer <ROOT_TOKEN> válido por
// um cookie de sessão de navegador (streamedia_session), permitindo navegar
// por /dashboard, /docs e /playground sem reenviar o header Authorization.
// Exige Bearer válido — não pode ser chamado a partir de uma sessão de cookie
// já existente.
func HandleSessionLogin(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validBearer(r, cfg.RootToken) {
			apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
			return
		}

		value, expiresAt := auth.IssueSessionToken(cfg.RootToken, cfg.SessionTTL)
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    value,
			Path:     "/",
			Expires:  expiresAt,
			MaxAge:   int(cfg.SessionTTL.Seconds()),
			HttpOnly: true,
			Secure:   cfg.SessionCookieSecure,
			SameSite: http.SameSiteStrictMode,
		})

		apiresponse.Success(w, http.StatusOK, sessionResponse{
			Active:    true,
			ExpiresAt: expiresAt.UTC().Format(http.TimeFormat),
		})
	}
}

// HandleSessionLogout apaga o cookie de sessão de navegador. Pública e
// idempotente (não exige RootAuth) — encerrar uma sessão inexistente ou já
// expirada não é um erro.
func HandleSessionLogout(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   cfg.SessionCookieSecure,
			SameSite: http.SameSiteStrictMode,
		})
		apiresponse.Success(w, http.StatusOK, sessionResponse{Active: false})
	}
}
