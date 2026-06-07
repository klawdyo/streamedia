// Pacote admin implementa as rotas de administração.
package admin

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// adminScopeContextKey é a chave usada para guardar, no contexto da
// requisição, o escopo de projeto resolvido pelo middleware AdminAuth
// (T33, issue #6): nil para super-admin (ADMIN_TOKEN global, sem
// restrição), ou o ID do projeto cuja chave mestra autenticou a requisição.
type adminScopeContextKey struct{}

// ProjectScopeFromContext devolve o ID do projeto ao qual a requisição
// admin está restrita, ou nil se a requisição for de um super-admin (sem
// escopo — enxerga todos os projetos). Use para filtrar consultas por
// project_id nos handlers de /admin/*.
func ProjectScopeFromContext(ctx context.Context) *int64 {
	scope, _ := ctx.Value(adminScopeContextKey{}).(*int64)
	return scope
}

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

// AdminAuth é um middleware que valida o token de administração no header
// Authorization: Bearer {token}. Usa ConstantTimeCompare para evitar timing attacks.
// Retorna 401 se o token estiver ausente ou incorreto.
//
// Decisão de modelo admin (T33, issue #6 — opção (a) do spec da tarefa):
// o ADMIN_TOKEN global continua funcionando como "super admin", sem
// restrição — preserva 100% o comportamento e a configuração existentes.
// Adicionalmente, a chave mestra de um projeto também autentica em
// /admin/*, mas com o escopo restrito aos vídeos daquele projeto (resolvido
// via models.GetProjectByMasterKeyHash e exposto aos handlers através de
// ProjectScopeFromContext). Optou-se por (a) em vez de (b) — substituir
// totalmente por chaves por projeto — por ser a migração mais simples e não
// quebrar instalações que já dependem só do ADMIN_TOKEN.
func AdminAuth(adminToken string, db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extrai o header Authorization
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
				return
			}

			// Espera o formato "Bearer {token}"
			const bearerPrefix = "Bearer "
			if !strings.HasPrefix(authHeader, bearerPrefix) {
				apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
				return
			}

			token := authHeader[len(bearerPrefix):]

			// 1. Super-admin: compara com o ADMIN_TOKEN global em tempo
			// constante. Sem escopo — vê e opera sobre todos os projetos.
			if adminToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}

			// 2. Admin de projeto: a própria chave mestra autentica — o
			// servidor calcula o hash e resolve o projeto, sem nunca reter a
			// chave em texto puro (mesmo princípio do X-Project-Key em
			// /upload/init). O escopo (project_id) é propagado no contexto
			// para os handlers filtrarem suas consultas.
			if project, err := models.GetProjectByMasterKeyHash(db, models.HashMasterKey(token)); err == nil {
				projectID := project.ID
				ctx := context.WithValue(r.Context(), adminScopeContextKey{}, &projectID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
		})
	}
}

// videosResponse é a estrutura de resposta para a rota de vídeos.
type videosResponse struct {
	Videos []*models.Video `json:"videos"`
	Total  int             `json:"total"`
}

// HandleVideos retorna uma lista paginada de vídeos, opcionalmente filtrada por status.
// Query params:
//   - status (opcional): filtro por status do vídeo
//   - limit (opcional, padrão 50, máximo 200): número de registros por página
//   - offset (opcional, padrão 0): número de registros a pular
//
// Retorna JSON com array de vídeos e contagem total.
func (h *AdminHandler) HandleVideos(w http.ResponseWriter, r *http.Request) {
	// Parse dos query parameters
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

	status := r.URL.Query().Get("status")

	// Monta a cláusula WHERE dinamicamente: filtro opcional por status e,
	// quando a requisição vem autenticada com a chave mestra de um projeto
	// (T33, issue #6), filtro obrigatório por project_id — restringe a
	// listagem aos vídeos daquele projeto ("não enxerga vídeos de outro").
	// Super-admin (ADMIN_TOKEN global) não tem escopo: vê todos os vídeos.
	var conditions []string
	var args []interface{}
	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if scope := ProjectScopeFromContext(r.Context()); scope != nil {
		conditions = append(conditions, "project_id = ?")
		args = append(args, *scope)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM videos" + whereClause
	listQuery := "SELECT video_id, status, declared_size_bytes, actual_size_bytes, duration_s, resolutions, transcode_attempts, last_chunk_at, error_message, project_id, created_at, updated_at FROM videos" +
		whereClause + " ORDER BY created_at DESC LIMIT ? OFFSET ?"

	// Conta o total de registros (mesmos filtros, sem LIMIT/OFFSET)
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao contar vídeos.")
		return
	}

	// Executa a query de listagem
	listArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := h.db.Query(listQuery, listArgs...)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao listar vídeos.")
		return
	}
	defer rows.Close()

	var videos []*models.Video
	for rows.Next() {
		var (
			v            models.Video
			declaredSize sql.NullInt64
			actualSize   sql.NullInt64
			durationS    sql.NullInt64
			resolutions  sql.NullString
			lastChunkAt  sql.NullTime
			errorMessage sql.NullString
			projectID    sql.NullInt64
		)

		err := rows.Scan(
			&v.VideoID,
			&v.Status,
			&declaredSize,
			&actualSize,
			&durationS,
			&resolutions,
			&v.TranscodeAttempts,
			&lastChunkAt,
			&errorMessage,
			&projectID,
			&v.CreatedAt,
			&v.UpdatedAt,
		)
		if err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao ler vídeos.")
			return
		}

		// Desserializa os campos nullable
		v.DeclaredSizeBytes = declaredSize.Int64
		v.ActualSizeBytes = actualSize.Int64
		v.DurationS = int(durationS.Int64)
		v.ErrorMessage = errorMessage.String
		if projectID.Valid {
			v.ProjectID = &projectID.Int64
		}

		if lastChunkAt.Valid {
			v.LastChunkAt = &lastChunkAt.Time
		}

		if resolutions.Valid && resolutions.String != "" {
			var res []int
			if err := json.Unmarshal([]byte(resolutions.String), &res); err == nil {
				v.Resolutions = res
			} else {
				v.Resolutions = []int{}
			}
		} else {
			v.Resolutions = []int{}
		}

		videos = append(videos, &v)
	}

		if err := rows.Err(); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao iterar vídeos.")
		return
	}

	// Se nenhum vídeo foi encontrado, retorna array vazio
	if videos == nil {
		videos = []*models.Video{}
	}

	// Monta a resposta JSON no envelope padrão.
	resp := videosResponse{
		Videos: videos,
		Total:  total,
	}
	apiresponse.Success(w, http.StatusOK, resp)
}

// queueResponse é a estrutura de resposta para a rota de fila.
type queueResponse struct {
	QueueLength int `json:"queue_length"`
	Workers     int `json:"workers"`
}

// HandleQueue retorna o estado atual da fila de transcodificação.
// Response:
//   {
//     "queue_length": 3,
//     "workers": 1
//   }
func (h *AdminHandler) HandleQueue(w http.ResponseWriter, r *http.Request) {
	resp := queueResponse{
		QueueLength: h.queue.Len(),
		Workers:     h.cfg.TranscodeWorkers,
	}

	apiresponse.Success(w, http.StatusOK, resp)
}
