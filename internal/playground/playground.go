// Pacote playground expõe uma interface web interativa (single-file, sem
// build step) — o "playground" da API do Streamedia — que exercita o
// pipeline completo de ponta a ponta: autenticação (ROOT_TOKEN) →
// upload/init → upload TUS em chunks → play/init → reprodução HLS por
// resolução. (issue #18)
//
// A rota é /playground — termo usual para um testador interativo de API.
//
// Os eventos do pipeline (upload concluído, transcodificação pronta, falha)
// são acompanhados ao vivo via SSE em GET /api/events, escopado por video_id
// e autenticado pelo token de upload. A página é apenas um cliente desse
// stream — não há mais receptor de webhooks em memória aqui.
package playground

import (
	"embed"
	"net/http"
)

// indexHTML é a página do playground, embutida no binário em tempo de build.
// Mantida como arquivo .html separado (e não string Go) para preservar o
// destaque de sintaxe do editor e a legibilidade — o navegador recebe um
// único HTML autocontido, sem bundler.
//
//go:embed index.html
var indexFS embed.FS

// Handler serve a página do playground.
type Handler struct{}

// NewHandler cria o Handler do playground.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeUI devolve a página HTML do playground (GET /playground).
func (h *Handler) ServeUI(w http.ResponseWriter, _ *http.Request) {
	page, err := indexFS.ReadFile("index.html")
	if err != nil {
		// O arquivo é embutido em build; um erro aqui indicaria um problema de
		// empacotamento, não uma condição de runtime.
		http.Error(w, "página do playground indisponível", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}
