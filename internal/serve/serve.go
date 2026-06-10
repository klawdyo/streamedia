// Pacote serve expõe as rotas de serving HLS, sob o prefixo /video/<tag>/...:
//
//  1. /video/<tag>/<video_id>.m3u8 — master playlist DINÂMICO: autenticado por
//     token de play (lookup em access_tokens) e pelo status do vídeo. O caminho
//     real no disco (<MEDIA_DIR>/<tag>/<video_id>/master.m3u8) fica escondido;
//     o handler reescreve as referências de variante para incluir o <video_id>.
//  2. /video/<tag>/<video_id>/<res>/playlist.m3u8 e .../<seg>.ts — ESTÁTICOS e
//     públicos (os nomes opacos no master funcionam como a "chave" de acesso).
package serve

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// uuidV4Re valida UUID de qualquer versão (1-8) — o video_id sempre é um UUID.
var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// allowedResolutions são as únicas resoluções aceitas no serving estático.
var allowedResolutions = map[string]bool{
	"480":  true,
	"720":  true,
	"1080": true,
}

// recordPlaybackAsync grava um evento de estatística de uso (T26/T27) sem
// bloquear a resposta ao cliente. Erros são apenas logados — estatísticas
// nunca devem afetar a entrega do conteúdo.
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

// videoPathParts remove o prefixo "/video/" do path e o divide por "/".
// NÃO normaliza o path — qualquer ".." é preservado para ser rejeitado depois.
func videoPathParts(urlPath string) []string {
	trimmed := strings.TrimPrefix(urlPath, "/video/")
	return strings.Split(trimmed, "/")
}

// MasterHandler serve o master.m3u8 autenticado por token de play e pelo
// status do vídeo.
type MasterHandler struct {
	cfg *config.Config
	db  *sql.DB

	// onStatsRecorded, se não-nil, é chamado ao final de cada gravação
	// assíncrona de evento — usado apenas em testes.
	onStatsRecorded func(error)
}

// NewMasterHandler cria um MasterHandler com a config e o banco informados.
func NewMasterHandler(cfg *config.Config, db *sql.DB) *MasterHandler {
	return &MasterHandler{cfg: cfg, db: db}
}

// ServeHTTP implementa o fluxo de serving do master.m3u8 dinâmico:
// /video/{tag}/{video_id}.m3u8?token=...
func (h *MasterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extrai tag e <video_id>.m3u8 do path.
	parts := videoPathParts(r.URL.Path)
	if len(parts) != 2 || !strings.HasSuffix(parts[1], ".m3u8") {
		apiresponse.Error(w, http.StatusBadRequest, "Caminho inválido.")
		return
	}
	urlTag := parts[0]
	videoID := strings.TrimSuffix(parts[1], ".m3u8")

	// 2. Valida o video_id como UUID — bloqueia path traversal por si só.
	if !uuidV4Re.MatchString(videoID) {
		apiresponse.Error(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	// 3. Valida o token de play: existe, é de propósito 'play', pertence a
	// este vídeo e não expirou (validação por lookup, sem HMAC).
	token := r.URL.Query().Get("token")
	at, err := models.GetAccessToken(h.db, token)
	if err != nil || at.Purpose != models.PurposePlay || at.VideoID != videoID || at.IsExpired() {
		apiresponse.Error(w, http.StatusUnauthorized, "Token de reprodução inválido ou expirado.")
		return
	}

	// 4. Busca o vídeo: exige status "ready" e que a tag da URL corresponda à
	// tag real (evita URLs com namespace incorreto).
	video, err := models.GetVideo(h.db, videoID)
	if err == sql.ErrNoRows {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao consultar o vídeo.")
		return
	}
	if video.Status != models.StatusReady {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não está disponível para reprodução.")
		return
	}
	if models.Slugify(urlTag) != video.Tag {
		apiresponse.Error(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}

	// 5. Registra o evento de playback (assíncrono, best-effort).
	recordPlaybackAsync(h.db, videoID, "playback", nil, r.Header.Get("User-Agent"), h.onStatsRecorded)

	// 6. Lê o master.m3u8 do disco e reescreve as referências de variante
	// para incluir o <video_id> (a URL pública é /video/<tag>/<id>.m3u8, então
	// "480/playlist.m3u8" precisa virar "<id>/480/playlist.m3u8" para resolver
	// em /video/<tag>/<id>/480/playlist.m3u8).
	masterPath := filepath.Join(h.cfg.MediaDir, video.Tag, videoID, "master.m3u8")
	content, err := os.ReadFile(masterPath)
	if err != nil {
		apiresponse.Error(w, http.StatusNotFound, "Master playlist não encontrado.")
		return
	}
	rewritten := rewriteMasterPlaylist(content, videoID)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	_, _ = w.Write(rewritten)
}

// rewriteMasterPlaylist prefixa cada linha de URI de variante (não-comentário,
// não-vazia) com "<video_id>/", deixando intactas as linhas de diretiva (#...).
func rewriteMasterPlaylist(content []byte, videoID string) []byte {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines[i] = videoID + "/" + trimmed
	}
	return []byte(strings.Join(lines, "\n"))
}

// StaticHandler serve playlists de resolução e segmentos .ts como arquivos
// estáticos públicos, com validação rígida de path e directory listing off.
type StaticHandler struct {
	cfg *config.Config
	db  *sql.DB

	// onStatsRecorded, se não-nil, é chamado ao final de cada gravação
	// assíncrona de evento — usado apenas em testes.
	onStatsRecorded func(error)
}

// NewStaticHandler cria um StaticHandler com a config e o banco informados.
// O banco é usado apenas para registrar eventos de estatística (T26/T27).
func NewStaticHandler(cfg *config.Config, db *sql.DB) *StaticHandler {
	return &StaticHandler{cfg: cfg, db: db}
}

// ServeHTTP implementa o fluxo de serving estático:
// /video/{tag}/{video_id}/{resolution}/{filename}
func (h *StaticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extrai os componentes do path.
	parts := videoPathParts(r.URL.Path)
	if len(parts) < 4 {
		apiresponse.Error(w, http.StatusBadRequest, "Caminho inválido.")
		return
	}
	tag := parts[0]
	videoID := parts[1]
	resolution := parts[2]
	// Qualquer ".." aparece aqui e é rejeitado pela validação de filename.
	filename := strings.Join(parts[3:], "/")

	// 2. Valida o video_id como UUID.
	if !uuidV4Re.MatchString(videoID) {
		apiresponse.Error(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	// 3. Valida a resolução contra a lista permitida.
	if !allowedResolutions[resolution] {
		apiresponse.Error(w, http.StatusBadRequest, "Resolução inválida.")
		return
	}

	// 4. Filename vazio (tentativa de listar diretório): 404.
	if filename == "" {
		apiresponse.Error(w, http.StatusNotFound, "Arquivo não encontrado.")
		return
	}

	// 5. Valida o filename: só "{dígitos}.ts" ou "playlist.m3u8".
	if filename != "playlist.m3u8" && !models.SegmentNameRe.MatchString(filename) {
		apiresponse.Error(w, http.StatusBadRequest, "Nome de arquivo inválido.")
		return
	}

	// 6. Resolve o path dentro do MediaDir a partir da tag (Slugify garante
	// segurança de path; não há lookup no banco — segmentos são públicos):
	// <MEDIA_DIR>/<tag>/<video_id>/<resolution>/<filename>.
	path := filepath.Join(h.cfg.MediaDir, models.Slugify(tag), videoID, resolution, filename)

	// 7. Proteção extra contra traversal: o path resolvido precisa estar
	// contido no MediaDir.
	mediaRoot := filepath.Clean(h.cfg.MediaDir)
	cleanPath := filepath.Clean(path)
	if cleanPath != mediaRoot && !strings.HasPrefix(cleanPath, mediaRoot+string(os.PathSeparator)) {
		apiresponse.Error(w, http.StatusBadRequest, "Caminho fora do diretório de mídia.")
		return
	}

	// 8. Directory listing off: se for diretório ou não existir, 404.
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			apiresponse.Error(w, http.StatusNotFound, "Arquivo não encontrado.")
			return
		}
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao acessar o arquivo.")
		return
	}
	if info.IsDir() {
		apiresponse.Error(w, http.StatusNotFound, "Arquivo não encontrado.")
		return
	}

	// 9. Registra o evento de estatística apenas para segmentos .ts.
	if models.SegmentNameRe.MatchString(filename) {
		if resInt, err := strconv.Atoi(resolution); err == nil {
			recordPlaybackAsync(h.db, videoID, "download_segment", &resInt, r.Header.Get("User-Agent"), h.onStatsRecorded)
		}
	}

	// 10. Serve o arquivo estático.
	http.ServeFile(w, r, cleanPath)
}
