// Package docs serve a documentação interativa da API: uma página HTML que
// carrega o componente Scalar via CDN e o arquivo de especificação OpenAPI.
//
// A especificação é escrita manualmente em openapi.json (não há geração
// automática a partir de comentários nas rotas — decisão registrada na
// issue #12 para manter o projeto sem dependências Go adicionais e com
// impacto desprezível no tamanho do binário).
package docs

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var docsPage []byte

//go:embed openapi.json
var openAPISpec []byte

// Handler serve as rotas públicas de documentação da API: a página /docs
// (UI do Scalar) e o JSON da especificação OpenAPI em /docs/openapi.json.
type Handler struct{}

// NewHandler cria um Handler de documentação. Não depende de config nem
// banco de dados — a especificação é estática e embutida no binário.
func NewHandler() *Handler {
	return &Handler{}
}

// ServePage responde GET /docs com a página HTML que carrega o Scalar via CDN.
func (h *Handler) ServePage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(docsPage)
}

// ServeSpec responde GET /docs/openapi.json com a especificação OpenAPI 3.x.
func (h *Handler) ServeSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}
