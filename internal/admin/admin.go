// Pacote admin implementa as rotas de administração, todas protegidas pelo
// ROOT_TOKEN único (Authorization: Bearer). Não há mais escopo por projeto/
// tenant: o único cliente privilegiado é o backend principal, que detém o
// ROOT_TOKEN e enxerga/opera sobre tudo.
package admin

import (
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

// RootAuth é o middleware que valida o ROOT_TOKEN no header
// Authorization: Bearer {token}. Usa comparação em tempo constante para
// prevenir timing attacks. Retorna 401 se o token estiver ausente ou incorreto.
//
// É a ÚNICA porta de autenticação de gestão do sistema: protege /api/upload/init,
// /api/play/init, /api/status e todas as rotas /admin/*.
func RootAuth(rootToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const bearerPrefix = "Bearer "
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, bearerPrefix) {
				apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
				return
			}
			token := authHeader[len(bearerPrefix):]
			if rootToken == "" || !auth.SecureCompare(token, rootToken) {
				apiresponse.Error(w, http.StatusUnauthorized, "Não autorizado.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// videosResponse é a estrutura de resposta para a rota de vídeos.
type videosResponse struct {
	Videos []*models.Video `json:"videos"`
	Total  int             `json:"total"`
}

// HandleVideos retorna uma lista paginada de vídeos, opcionalmente filtrada por
// status e/ou tag.
// Query params:
//   - status (opcional): filtro por status do vídeo
//   - tag (opcional): filtro por namespace (tag)
//   - limit (opcional, padrão 50, máximo 200): número de registros por página
//   - offset (opcional, padrão 0): número de registros a pular
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

	countQuery := "SELECT COUNT(*) FROM videos" + whereClause
	listQuery := "SELECT " + models.SelectVideoColumns + " FROM videos" +
		whereClause + " ORDER BY created_at DESC LIMIT ? OFFSET ?"

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
