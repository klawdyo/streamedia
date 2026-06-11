// Pacote dashboard expõe a área administrativa visual do Streamedia: uma
// visão geral do sistema (estatísticas, fila, espaço usado, gráficos de
// movimentação), a biblioteca de vídeos (lista com paginação, filtros e
// ordenação) e a página de um vídeo (player estilo YouTube + estatísticas).
//
// Padrão de autenticação (igual ao /playground): as PÁGINAS HTML são públicas
// — não fazem nada de útil sem o ROOT_TOKEN. O token é colado uma vez pelo
// usuário, guardado no sessionStorage do navegador e enviado em
// `Authorization: Bearer` a cada chamada das rotas protegidas (/admin/*,
// /api/status, /api/play/init). Nenhum dado vaza pela casca pública: ele só
// chega via essas rotas, que continuam exigindo o ROOT_TOKEN no servidor.
//
// As páginas são HTML autocontido (sem build step), carregando Chart.js e
// hls.js via CDN — mesma decisão do /playground (hls.js) e do /docs (Scalar).
// O tema visual reaproveita as variáveis de cor "inspiradas no Scalar" do
// playground, extraídas para assets/theme.css.
package dashboard

import (
	"embed"
	"net/http"
	"path"
	"strings"
)

// contentFS embute as páginas e os assets do dashboard no binário em tempo de
// build — sem dependência de arquivos externos no ambiente de execução.
//
//go:embed overview.html videos.html video.html assets/*
var contentFS embed.FS

// Handler serve as páginas do dashboard e seus assets estáticos.
type Handler struct{}

// NewHandler cria o Handler do dashboard.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeOverview serve a visão geral (GET /dashboard).
func (h *Handler) ServeOverview(w http.ResponseWriter, _ *http.Request) {
	h.servePage(w, "overview.html")
}

// ServeVideos serve a biblioteca de vídeos (GET /dashboard/videos).
func (h *Handler) ServeVideos(w http.ResponseWriter, _ *http.Request) {
	h.servePage(w, "videos.html")
}

// ServeVideo serve a página de um vídeo (GET /dashboard/videos/{videoID}). O
// HTML é o mesmo para qualquer vídeo: o id é lido da URL pelo próprio JS da
// página, que então busca os dados nas rotas protegidas.
func (h *Handler) ServeVideo(w http.ResponseWriter, _ *http.Request) {
	h.servePage(w, "video.html")
}

// servePage escreve uma página HTML embutida no envelope HTML padrão.
func (h *Handler) servePage(w http.ResponseWriter, name string) {
	page, err := contentFS.ReadFile(name)
	if err != nil {
		// Os arquivos são embutidos em build; um erro aqui indicaria um
		// problema de empacotamento, não uma condição de runtime.
		http.Error(w, "página do dashboard indisponível", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

// ServeAsset serve um asset estático do dashboard (GET
// /dashboard/assets/{file}), como theme.css e app.js. O nome é restringido ao
// diretório assets/ e sem componentes de path (sem traversal): só o nome-base
// do arquivo é usado.
func (h *Handler) ServeAsset(w http.ResponseWriter, r *http.Request) {
	// path.Base descarta qualquer "../" — só resta o nome do arquivo.
	name := path.Base(r.URL.Path)
	data, err := contentFS.ReadFile("assets/" + name)
	if err != nil {
		http.Error(w, "asset não encontrado", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType(name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// contentType devolve o Content-Type a partir da extensão do arquivo. Mantido
// explícito (sem mime.TypeByExtension) para não depender do registro de tipos
// do SO, que varia entre ambientes.
func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}
