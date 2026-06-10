package serve

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// ThumbnailHandler serve os thumbnails (posters) JPEG de cada resolução como
// arquivos estáticos PÚBLICOS — sem autenticação, por serem capas de natureza
// pública (issue #19). Path no disco:
// <MEDIA_DIR>/<tag>/<video_id>/thumb_<res>.jpg, exposto na rota pública
// /video/<tag>/<video_id>/thumb_<res>.jpg.
//
// Não depende do banco: o nome opaco (UUID do vídeo) já funciona como "chave"
// de acesso, o mesmo critério dos segmentos .ts servidos pelo StaticHandler.
type ThumbnailHandler struct {
	cfg *config.Config
}

// NewThumbnailHandler cria um ThumbnailHandler com a config informada.
func NewThumbnailHandler(cfg *config.Config) *ThumbnailHandler {
	return &ThumbnailHandler{cfg: cfg}
}

// ServeHTTP implementa o serving estático do thumbnail:
// /video/{tag}/{video_id}/{thumb_<res>.jpg}
func (h *ThumbnailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extrai tag, video_id e filename do path (3 componentes após /video/).
	parts := videoPathParts(r.URL.Path)
	if len(parts) != 3 {
		apiresponse.Error(w, http.StatusBadRequest, "Caminho inválido.")
		return
	}
	tag := parts[0]
	videoID := parts[1]
	filename := parts[2]

	// 2. Valida o video_id como UUID — bloqueia path traversal por si só.
	if !uuidV4Re.MatchString(videoID) {
		apiresponse.Error(w, http.StatusBadRequest, "video_id inválido.")
		return
	}

	// 3. Valida o filename: só "thumb_<res>.jpg" das resoluções permitidas.
	if !models.ThumbnailNameRe.MatchString(filename) {
		apiresponse.Error(w, http.StatusBadRequest, "Nome de arquivo inválido.")
		return
	}

	// 4. Resolve o path dentro do MediaDir (Slugify garante segurança de path):
	// <MEDIA_DIR>/<tag>/<video_id>/<filename>.
	path := filepath.Join(h.cfg.MediaDir, models.Slugify(tag), videoID, filename)

	// 5. Proteção extra contra traversal: o path resolvido precisa estar
	// contido no MediaDir.
	mediaRoot := filepath.Clean(h.cfg.MediaDir)
	cleanPath := filepath.Clean(path)
	if cleanPath != mediaRoot && !strings.HasPrefix(cleanPath, mediaRoot+string(os.PathSeparator)) {
		apiresponse.Error(w, http.StatusBadRequest, "Caminho fora do diretório de mídia.")
		return
	}

	// 6. Directory listing off: se for diretório ou não existir, 404.
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			apiresponse.Error(w, http.StatusNotFound, "Thumbnail não encontrado.")
			return
		}
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao acessar o thumbnail.")
		return
	}
	if info.IsDir() {
		apiresponse.Error(w, http.StatusNotFound, "Thumbnail não encontrado.")
		return
	}

	// 7. Serve o arquivo (http.ServeFile define o Content-Type pela extensão).
	http.ServeFile(w, r, cleanPath)
}
