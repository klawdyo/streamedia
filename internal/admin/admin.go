// Pacote admin implementa as rotas de administração.
package admin

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

// AdminAuth é um middleware que valida o token de administração no header
// Authorization: Bearer {token}. Usa ConstantTimeCompare para evitar timing attacks.
// Retorna 401 se o token estiver ausente ou incorreto.
func AdminAuth(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extrai o header Authorization
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Espera o formato "Bearer {token}"
			const bearerPrefix = "Bearer "
			if !strings.HasPrefix(authHeader, bearerPrefix) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			token := authHeader[len(bearerPrefix):]

			// Usa ConstantTimeCompare para evitar timing attacks
			if subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
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

	// Monta a consulta SQL com a cláusula WHERE condicional
	var countQuery string
	var listQuery string
	var args []interface{}

	if status != "" {
		countQuery = "SELECT COUNT(*) FROM videos WHERE status = ?"
		listQuery = "SELECT video_id, status, declared_size_bytes, actual_size_bytes, duration_s, resolutions, transcode_attempts, last_chunk_at, error_message, created_at, updated_at FROM videos WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = []interface{}{status, limit, offset}
	} else {
		countQuery = "SELECT COUNT(*) FROM videos"
		listQuery = "SELECT video_id, status, declared_size_bytes, actual_size_bytes, duration_s, resolutions, transcode_attempts, last_chunk_at, error_message, created_at, updated_at FROM videos ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = []interface{}{limit, offset}
	}

	// Conta o total de registros
	var total int
	var countArgs []interface{}
	if status != "" {
		countArgs = []interface{}{status}
	}
	if err := h.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		http.Error(w, "Erro ao contar vídeos", http.StatusInternalServerError)
		return
	}

	// Executa a query de listagem
	rows, err := h.db.Query(listQuery, args...)
	if err != nil {
		http.Error(w, "Erro ao listar vídeos", http.StatusInternalServerError)
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
			&v.CreatedAt,
			&v.UpdatedAt,
		)
		if err != nil {
			http.Error(w, "Erro ao ler vídeos", http.StatusInternalServerError)
			return
		}

		// Desserializa os campos nullable
		v.DeclaredSizeBytes = declaredSize.Int64
		v.ActualSizeBytes = actualSize.Int64
		v.DurationS = int(durationS.Int64)
		v.ErrorMessage = errorMessage.String

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
		http.Error(w, "Erro ao iterar vídeos", http.StatusInternalServerError)
		return
	}

	// Se nenhum vídeo foi encontrado, retorna array vazio
	if videos == nil {
		videos = []*models.Video{}
	}

	// Monta a resposta JSON
	resp := videosResponse{
		Videos: videos,
		Total:  total,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Log silencioso de erros de encoding - cliente já desconectou
	}
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Log silencioso de erros de encoding - cliente já desconectou
	}
}
