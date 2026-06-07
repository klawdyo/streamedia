// Pacote serve expõe as rotas de serving HLS.
//
// Existem dois tipos de serving:
//
//  1. master.m3u8 — autenticado por token HMAC de reprodução e pelo status
//     do vídeo no banco.
//  2. Playlists de resolução e segmentos .ts — estáticos e públicos (os nomes
//     opacos contidos no master.m3u8 funcionam como a "chave" de acesso).
//
// Toda extração de path é feita manualmente via strings.Split porque o chi
// ainda não está conectado nesta etapa (T12); ele entra em T20.
package serve

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/klawdyo/streamedia/internal/auth"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// uuidV4Re valida UUID de qualquer versão (1-8) — substituído pela função
// centralizada models.IsValidVideoIDFormat. Mantido como alias para evitar
// reescrever todos os call sites; a definição real está em internal/models.
var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// allowedResolutions são as únicas resoluções aceitas no serving estático.
var allowedResolutions = map[string]bool{
	"480":  true,
	"720":  true,
	"1080": true,
}

// segmentRe casa nomes de segmento: um ou mais dígitos seguidos de ".ts".
var segmentRe = regexp.MustCompile(`^[0-9]+\.ts$`)

// respondError escreve uma resposta JSON de erro com o status informado.
// Mensagens em português, conforme convenção do projeto para a API.
func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// Ignoramos o erro de Encode: o header e o status já foram enviados.
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// recordPlaybackAsync grava um evento de estatística de uso (T26/T27) sem
// bloquear a resposta ao cliente: a gravação ocorre em uma goroutine separada
// e qualquer erro é apenas logado — estatísticas nunca devem afetar a entrega
// do conteúdo ao usuário.
//
// onDone, se não-nil, é chamado ao final da gravação (com o erro, se houver).
// Existe apenas para permitir que os testes aguardem deterministicamente a
// conclusão da goroutine antes de inspecionar o banco — em produção é nil.
func recordPlaybackAsync(db *sql.DB, videoID, eventType string, resolution *int, userAgent string, onDone func(error)) {
	go func() {
		err := models.RecordEvent(db, videoID, eventType, resolution, userAgent)
		if err != nil {
			log.Printf("[stats] erro ao registrar evento %q para vídeo %s: %v", eventType, videoID, err)
		}
		if onDone != nil {
			onDone(err)
		}
	}()
}

// pathParts remove o prefixo "/videos/" do path e o divide por "/".
// Retorna os componentes não vazios já com o prefixo retirado.
//
// Importante: NÃO normalizamos o path aqui — qualquer ".." presente é
// preservado para que a validação subsequente possa rejeitá-lo.
func pathParts(urlPath string) []string {
	trimmed := strings.TrimPrefix(urlPath, "/videos/")
	return strings.Split(trimmed, "/")
}

// MasterHandler serve o arquivo master.m3u8 autenticado por token de
// reprodução e protegido pela verificação de status do vídeo.
type MasterHandler struct {
	cfg *config.Config
	db  *sql.DB

	// onStatsRecorded, se não-nil, é chamado ao final de cada gravação
	// assíncrona de evento de estatística. Usado apenas em testes para
	// aguardar deterministicamente a goroutine antes de inspecionar o banco.
	onStatsRecorded func(error)
}

// NewMasterHandler cria um MasterHandler com a config e o banco informados.
func NewMasterHandler(cfg *config.Config, db *sql.DB) *MasterHandler {
	return &MasterHandler{cfg: cfg, db: db}
}

// ServeHTTP implementa o fluxo de serving do master.m3u8.
func (h *MasterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extrai o video_id do path: /videos/{videoID}/master.m3u8
	parts := pathParts(r.URL.Path)
	if len(parts) < 2 {
		respondError(w, http.StatusBadRequest, "Caminho inválido.")
		return
	}
	videoID := parts[0]

	// 2. Valida o video_id como UUID v4 estrito. Isso, por si só, já bloqueia
	// qualquer tentativa de path traversal (ex.: "../etc/passwd").
	if !uuidV4Re.MatchString(videoID) {
		respondError(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	// 3. Extrai expires e token dos query params.
	q := r.URL.Query()
	expires, err := strconv.ParseInt(q.Get("expires"), 10, 64)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Parâmetro expires inválido.")
		return
	}
	token := q.Get("token")

	// 4. Valida o token de reprodução (expiração, TTL máximo e assinatura HMAC).
	// O secret de reprodução é o secret HMAC compartilhado (UPLOAD_TOKEN_SECRET).
	if err := auth.ValidatePlayToken(h.cfg.UploadTokenSecret, videoID, expires, token, h.cfg.PlayTokenMaxTTL); err != nil {
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// 5. Busca o vídeo no banco e exige status "ready".
	var status string
	var projectID sql.NullInt64
	err = h.db.QueryRow("SELECT status, project_id FROM videos WHERE video_id = ?", videoID).Scan(&status, &projectID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Erro ao consultar o vídeo.")
		return
	}
	if status != "ready" {
		respondError(w, http.StatusNotFound, "Vídeo não está disponível para reprodução.")
		return
	}

	// 6. Registra o evento de estatística (T26/T27): acesso ao master.m3u8
	// conta como "playback" sem resolução associada (o master não tem uma).
	// Gravado de forma assíncrona para não atrasar a entrega do conteúdo.
	recordPlaybackAsync(h.db, videoID, "playback", nil, r.Header.Get("User-Agent"), h.onStatsRecorded)

	// 7. Resolve o diretório raiz do projeto (issue #6, T34) e serve o
	// master.m3u8 de <MEDIA_DIR>/<slug-do-projeto>/<video_id>/master.m3u8.
	rootDir, err := models.ResolveVideoRootDir(h.db, nullableInt64(projectID))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Erro ao resolver o diretório do projeto.")
		return
	}
	masterPath := filepath.Join(h.cfg.MediaDir, rootDir, videoID, "master.m3u8")
	http.ServeFile(w, r, masterPath)
}

// nullableInt64 converte um sql.NullInt64 em *int64 (nil se NULL) — usado
// para repassar project_id a models.ResolveVideoRootDir.
func nullableInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// StaticHandler serve playlists de resolução e segmentos .ts como arquivos
// estáticos públicos, sem autenticação, mas com validação rígida de path e
// directory listing desabilitado.
type StaticHandler struct {
	cfg *config.Config
	db  *sql.DB

	// onStatsRecorded, se não-nil, é chamado ao final de cada gravação
	// assíncrona de evento de estatística. Usado apenas em testes para
	// aguardar deterministicamente a goroutine antes de inspecionar o banco.
	onStatsRecorded func(error)
}

// NewStaticHandler cria um StaticHandler com a config e o banco informados.
// O banco é usado apenas para registrar eventos de estatística (T26/T27).
func NewStaticHandler(cfg *config.Config, db *sql.DB) *StaticHandler {
	return &StaticHandler{cfg: cfg, db: db}
}

// ServeHTTP implementa o fluxo de serving estático.
func (h *StaticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extrai os componentes do path: /videos/{videoID}/{resolution}/{filename}
	parts := pathParts(r.URL.Path)
	if len(parts) < 3 {
		respondError(w, http.StatusBadRequest, "Caminho inválido.")
		return
	}
	videoID := parts[0]
	resolution := parts[1]
	// O filename é o restante reunido; assim, qualquer ".." no caminho aparece
	// aqui e é rejeitado pela validação de filename abaixo.
	filename := strings.Join(parts[2:], "/")

	// 2. Valida o video_id como UUID v4 estrito.
	if !uuidV4Re.MatchString(videoID) {
		respondError(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	// 3. Valida a resolução contra a lista permitida.
	if !allowedResolutions[resolution] {
		respondError(w, http.StatusBadRequest, "Resolução inválida.")
		return
	}

	// 4. Filename vazio (path terminando em "/", ex.: /videos/{id}/480/) é uma
	// tentativa de listar o diretório da resolução. Nunca listamos: 404.
	if filename == "" {
		respondError(w, http.StatusNotFound, "Arquivo não encontrado.")
		return
	}

	// 5. Valida o filename: só segmentos "{digitos}.ts" ou "playlist.m3u8".
	// Qualquer ".." ou barra extra reprova aqui.
	if filename != "playlist.m3u8" && !segmentRe.MatchString(filename) {
		respondError(w, http.StatusBadRequest, "Nome de arquivo inválido.")
		return
	}

	// 5. Resolve o diretório raiz do projeto do vídeo (issue #6, T34) e
	// constrói o path final dentro do MediaDir:
	// <MEDIA_DIR>/<slug-do-projeto>/<video_id>/<resolution>/<filename>.
	// Vídeo inexistente aqui não é erro de autorização — os segmentos são
	// públicos e "opacos"; tratamos como arquivo não encontrado (404),
	// igual a qualquer outro caminho inválido nesta rota.
	var projectID sql.NullInt64
	err := h.db.QueryRow("SELECT project_id FROM videos WHERE video_id = ?", videoID).Scan(&projectID)
	if err != nil && err != sql.ErrNoRows {
		respondError(w, http.StatusInternalServerError, "Erro ao consultar o vídeo.")
		return
	}
	rootDir, err := models.ResolveVideoRootDir(h.db, nullableInt64(projectID))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Erro ao resolver o diretório do projeto.")
		return
	}
	path := filepath.Join(h.cfg.MediaDir, rootDir, videoID, resolution, filename)

	// 6. Proteção extra contra traversal: o path resolvido precisa estar
	// contido no MediaDir. Usamos filepath.Clean nas duas pontas.
	mediaRoot := filepath.Clean(h.cfg.MediaDir)
	cleanPath := filepath.Clean(path)
	if cleanPath != mediaRoot && !strings.HasPrefix(cleanPath, mediaRoot+string(os.PathSeparator)) {
		respondError(w, http.StatusBadRequest, "Caminho fora do diretório de mídia.")
		return
	}

	// 7. Directory listing desabilitado: se o alvo for um diretório, 404.
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "Arquivo não encontrado.")
			return
		}
		respondError(w, http.StatusInternalServerError, "Erro ao acessar o arquivo.")
		return
	}
	if info.IsDir() {
		respondError(w, http.StatusNotFound, "Arquivo não encontrado.")
		return
	}

	// 8. Registra o evento de estatística (T26/T27) apenas para segmentos
	// .ts — é o evento mais representativo de consumo real (download de
	// dados de vídeo). Não registramos playlist.m3u8 aqui para evitar
	// duplicidade com o evento "playback" já gerado no acesso ao master.
	// Gravado de forma assíncrona para não atrasar a entrega do arquivo.
	if segmentRe.MatchString(filename) {
		resInt, err := strconv.Atoi(resolution)
		if err == nil {
			recordPlaybackAsync(h.db, videoID, "download_segment", &resInt, r.Header.Get("User-Agent"), h.onStatsRecorded)
		}
	}

	// 9. Serve o arquivo estático.
	http.ServeFile(w, r, cleanPath)
}
