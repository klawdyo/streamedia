// Pacote admin implementa as rotas de administração, todas protegidas pelo
// ROOT_TOKEN único (Authorization: Bearer). Não há mais escopo por projeto/
// tenant: o único cliente privilegiado é o backend principal, que detém o
// ROOT_TOKEN e enxerga/opera sobre tudo.
package admin

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/httputil"
	"github.com/klawdyo/streamedia/internal/models"
)

// AdminHandler agrega as dependências necessárias para as rotas de administração.
type AdminHandler struct {
	cfg   *config.Config
	db    *sql.DB
	queue interface{ Len() int }
}

// NewAdminHandler cria uma nova instância de AdminHandler com as dependências
// injetadas.
func NewAdminHandler(cfg *config.Config, db *sql.DB, queue interface{ Len() int }) *AdminHandler {
	return &AdminHandler{
		cfg:   cfg,
		db:    db,
		queue: queue,
	}
}

// RootAuth é o middleware que valida o acesso de gestão. Aceita DUAS formas
// de autenticação:
//
//  1. Authorization: Bearer <ROOT_TOKEN> — uso backend-to-backend (igual ao
//     modelo original). Comparação em tempo constante via auth.SecureCompare.
//  2. Cookie streamedia_session — sessão de navegador emitida por
//     POST /admin/session (ver internal/admin/session.go), validada por
//     auth.ValidateSessionToken. Como o cookie pode ser enviado
//     automaticamente pelo navegador em requisições cross-site, métodos não
//     seguros (POST/PUT/PATCH/DELETE) autenticados via cookie exigem também o
//     header X-Streamedia-Csrf — defesa em profundidade além do
//     SameSite=Strict do cookie. Requisições autenticadas via Bearer não
//     precisam desse header (não são suscetíveis a CSRF).
//
// Retorna 401 se nenhuma das duas autenticações for válida, e 403 se a
// sessão via cookie for válida mas faltar o header CSRF em método não seguro.
//
// É a ÚNICA porta de autenticação de gestão do sistema: protege
// /api/upload/init, /api/play/init, /api/status, /docs, /metrics e todas as
// rotas /admin/*.
func RootAuth(rootToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validBearer(r, rootToken) {
				next.ServeHTTP(w, r)
				return
			}

		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || rootToken == "" {
			apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
			return
		}

		// Tenta o formato novo primeiro (com user_id); se não for, cai
		// no formato antigo como fallback. Ambos usam o mesmo nome de
		// cookie — o formato é detectado pela quantidade de partes.
		_, userID, _, ok := auth.ValidateSessionTokenWithUser(rootToken, cookie.Value)
		if ok {
			// Formato novo: injeta userID no contexto para que handlers
			// como HandleMe possam identificar o usuário logado.
			if !isSafeMethod(r.Method) && r.Header.Get(CSRFHeaderName) != csrfHeaderValue {
				apiresponse.Error(w, http.StatusForbidden, "Requisição bloqueada: cabeçalho CSRF ausente.")
				return
			}
			ctx := context.WithValue(r.Context(), auth.UserIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fallback: formato antigo (sem user_id).
		if !auth.ValidateSessionToken(rootToken, cookie.Value) {
			apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
			return
		}

		if !isSafeMethod(r.Method) && r.Header.Get(CSRFHeaderName) != csrfHeaderValue {
			apiresponse.Error(w, http.StatusForbidden, "Requisição bloqueada: cabeçalho CSRF ausente.")
			return
		}

		next.ServeHTTP(w, r)
		})
	}
}

// validBearer confere o header Authorization: Bearer <ROOT_TOKEN> em tempo
// constante. Compartilhado por RootAuth e HandleSessionLogin.
func validBearer(r *http.Request, rootToken string) bool {
	const bearerPrefix = "Bearer "
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return false
	}
	token := authHeader[len(bearerPrefix):]
	return rootToken != "" && auth.SecureCompare(token, rootToken)
}

// isSafeMethod indica se o método HTTP não tem efeitos colaterais (não exige
// o header X-Streamedia-Csrf quando autenticado via cookie de sessão).
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// videosResponse é a estrutura de resposta para a rota de vídeos.
type videosResponse struct {
	Videos []*models.Video `json:"videos"`
	Total  int             `json:"total"`
}

// sortableVideoColumns mapeia o valor aceito no query param `sort` à coluna
// real usada no ORDER BY. É uma WHITELIST: nomes de coluna não podem ser
// parametrizados em SQL (só valores), então só interpolamos um nome que esteja
// aqui — qualquer outro valor cai no default, evitando injeção de SQL.
var sortableVideoColumns = map[string]string{
	"created_at":        "created_at",
	"updated_at":        "updated_at",
	"status":            "status",
	"actual_size_bytes": "actual_size_bytes",
	"duration_s":        "duration_s",
}

// HandleVideos retorna uma lista paginada de vídeos, opcionalmente filtrada por
// status e/ou tag, com ordenação configurável.
// Query params:
//   - status (opcional): filtro por status do vídeo
//   - tag (opcional): filtro por namespace (tag)
//   - limit (opcional, padrão 50, máximo 200): número de registros por página
//   - offset (opcional, padrão 0): número de registros a pular
//   - sort (opcional, padrão created_at): coluna de ordenação; só aceita os
//     valores da whitelist sortableVideoColumns, demais caem no default
//   - order (opcional, padrão desc): direção da ordenação (asc|desc)
func (h *AdminHandler) HandleVideos(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			if parsed > 200 {
				limit = 200
			} else if parsed > 0 {
				limit = parsed
			}
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Monta a cláusula WHERE dinamicamente: filtros opcionais por status e tag.
	var conditions []string
	var args []interface{}
	if status := r.URL.Query().Get("status"); status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		conditions = append(conditions, "tag = ?")
		args = append(args, models.Slugify(tag))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Ordenação: coluna pela whitelist (default created_at) e direção asc|desc
	// (default desc). Como o nome da coluna não pode ser parametrizado, só
	// interpolamos valores já validados — nunca o input cru do cliente.
	sortColumn := "created_at"
	if col, ok := sortableVideoColumns[r.URL.Query().Get("sort")]; ok {
		sortColumn = col
	}
	orderDir := "DESC"
	if strings.EqualFold(r.URL.Query().Get("order"), "asc") {
		orderDir = "ASC"
	}

	countQuery := "SELECT COUNT(*) FROM videos" + whereClause
	listQuery := "SELECT " + models.SelectVideoColumns + " FROM videos" +
		whereClause + " ORDER BY " + sortColumn + " " + orderDir + " LIMIT ? OFFSET ?"

	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao contar vídeos.")
		return
	}

	listArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := h.db.Query(listQuery, listArgs...)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao listar vídeos.")
		return
	}
	defer rows.Close()

	var videos []*models.Video
	for rows.Next() {
		v, err := models.ScanVideoRow(rows.Scan)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao ler vídeos.")
			return
		}
		videos = append(videos, v)
	}
	if err := rows.Err(); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao iterar vídeos.")
		return
	}
	if videos == nil {
		videos = []*models.Video{}
	}

	// Popula HasThumbnails e ThumbnailURL para cada vídeo na lista, verificando
	// o disco em <MEDIA_DIR>/<tag>/<video_id>/thumb_*.jpg. Para a lista, usamos
	// o menor thumbnail disponível (primeiro encontrado na ordem das resolutions)
	// como preview; se não houver nenhum, ThumbnailURL fica vazio e o frontend
	// mostra um placeholder.
	for _, v := range videos {
		basePath := filepath.Join(h.cfg.MediaDir, models.Slugify(v.Tag), v.VideoID)
		// Itera as resolutions do vídeo (se houver) e procura o primeiro thumbnail existente.
		for _, res := range v.Resolutions {
			thumbPath := filepath.Join(basePath, models.ThumbnailFileName(res))
			if info, err := os.Stat(thumbPath); err == nil && !info.IsDir() {
				v.HasThumbnails = true
				v.ThumbnailURL = httputil.PublicThumbnailURL(r, v.Tag, v.VideoID, res)
				break
			}
		}
	}

	apiresponse.Success(w, http.StatusOK, videosResponse{Videos: videos, Total: total})
}

// queueResponse é a estrutura de resposta para a rota de fila.
type queueResponse struct {
	QueueLength int `json:"queue_length"`
	Workers     int `json:"workers"`
}

// HandleQueue retorna o estado atual da fila de transcodificação.
func (h *AdminHandler) HandleQueue(w http.ResponseWriter, r *http.Request) {
	apiresponse.Success(w, http.StatusOK, queueResponse{
		QueueLength: h.queue.Len(),
		Workers:     h.cfg.TranscodeWorkers,
	})
}

// HandleDeleteVideo apaga um vídeo: remove suas linhas no banco (tokens de
// acesso, variantes, eventos e o próprio vídeo) e o diretório de arquivos no
// disco (<MEDIA_DIR>/<tag>/<video_id>). Operação de gestão — protegida pelo
// ROOT_TOKEN (middleware RootAuth).
func (h *AdminHandler) HandleDeleteVideo(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "videoID")

	video, err := models.GetVideo(h.db, videoID)
	if err == sql.ErrNoRows {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao consultar o vídeo.")
		return
	}

	// Remove dependentes (FK) e o vídeo. Eventos de playback não têm FK, mas
	// também são removidos para não deixar estatísticas órfãs.
	if err := models.DeleteAccessTokensForVideo(h.db, videoID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao remover tokens do vídeo.")
		return
	}
	if _, err := h.db.Exec("DELETE FROM video_renditions WHERE video_id = ?", videoID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao remover variantes do vídeo.")
		return
	}
	if _, err := h.db.Exec("DELETE FROM playback_events WHERE video_id = ?", videoID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao remover eventos do vídeo.")
		return
	}
	if err := models.DeleteVideo(h.db, videoID); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao remover o vídeo.")
		return
	}

	// Remove os arquivos do disco. A tag já está normalizada (Slugify no
	// upload-init), mas reaplicamos por garantia contra qualquer valor herdado.
	videoDir := filepath.Join(h.cfg.MediaDir, models.Slugify(video.Tag), videoID)
	if err := os.RemoveAll(videoDir); err != nil {
		// Linhas já removidas — apenas reporta o resíduo no disco.
		apiresponse.Error(w, http.StatusInternalServerError, "Vídeo removido do banco, mas houve erro ao apagar os arquivos.")
		return
	}

	apiresponse.Success(w, http.StatusOK, map[string]string{"video_id": videoID, "deleted": "true"})
}

// RoleAuth é o middleware que valida se o usuário autenticado possui pelo menos
// UMA das roles listadas no parâmetro allowedRoles. Deve ser usado APÓS o
// RootAuth (que já validou a autenticação e injetou o userID no contexto).
//
// Se a requisição foi autenticada via Bearer ROOT_TOKEN (sem userID no
// contexto), RoleAuth permite o acesso automaticamente — o ROOT_TOKEN é o
// mecanismo de backend-to-backend (scraper Prometheus, CI/CD) e tem acesso
// total a tudo que o middleware RootAuth já liberou.
//
// Uso:
//
//	r.Group(func(r chi.Router) {
//	    r.Use(admin.RootAuth(cfg.RootToken))
//	    r.Use(admin.RoleAuth(database, "admin", "acl", "manager"))
//	    r.Get("/admin/videos", adminHandler.HandleVideos)
//	})
func RoleAuth(db *sql.DB, allowedRoles ...string) func(http.Handler) http.Handler {
	// Converte a lista para um mapa para lookup O(1) em cada requisição.
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Se autenticado via Bearer ROOT_TOKEN (sem userID), acesso total.
			userID, ok := auth.GetUserIDFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			// Busca as roles do usuário no banco.
			roles, err := models.GetUserRoles(db, userID)
			if err != nil {
				apiresponse.Error(w, http.StatusInternalServerError, "Erro ao verificar permissões.")
				return
			}

			// Verifica se o usuário tem pelo menos uma das roles permitidas.
			for _, role := range roles {
				if allowed[role.Role] {
					next.ServeHTTP(w, r)
					return
				}
			}

			apiresponse.Error(w, http.StatusForbidden, "Acesso negado: você não tem permissão para esta operação.")
		})
	}
}
