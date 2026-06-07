// Pacote docs expõe a documentação interativa da API HTTP do Streamedia
// (issue #3 / T30): uma especificação OpenAPI servida em JSON e uma UI
// Swagger que a consome, ambas acessíveis via navegador em /docs/.
//
// Decisão de autenticação: a documentação fica SEM autenticação, no mesmo
// espírito da rota /metrics (T29) — é material de referência para quem
// integra com a API (incluindo as rotas administrativas, que continuam
// protegidas por AdminAuth/HMAC nas próprias rotas reais; a spec apenas
// descreve seus contratos, não dá acesso a elas). O rate limiter global
// (T19) já mitiga abuso de scraping da documentação.
package docs

import (
	"encoding/json"
	"net/http"
)

// Handler agrega o necessário para servir a documentação da API: a spec
// OpenAPI pré-serializada (gerada uma vez, na construção) e a UI estática
// que a renderiza.
type Handler struct {
	specJSON []byte
}

// NewHandler cria um Handler com a especificação OpenAPI da API Streamedia
// já serializada. A spec é fixa em tempo de build (não depende de runtime),
// por isso é montada uma única vez aqui.
func NewHandler() *Handler {
	spec, err := json.MarshalIndent(openAPISpec(), "", "  ")
	if err != nil {
		// A spec é um literal estático desta função; um erro aqui indicaria
		// um bug de programação (valor não serializável), não uma condição
		// de runtime — por isso entra em pânico em vez de propagar erro.
		panic("docs: falha ao serializar especificação OpenAPI: " + err.Error())
	}
	return &Handler{specJSON: spec}
}

// ServeOpenAPISpec devolve a especificação OpenAPI em JSON (GET /docs/openapi.json).
func (h *Handler) ServeOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.specJSON)
}

// ServeUI devolve uma página HTML que carrega o Swagger UI (via CDN) apontando
// para a spec servida em /docs/openapi.json (GET /docs/ ou /docs).
//
// Optamos por carregar os assets do Swagger UI via CDN em vez de embuti-los
// no binário: evita inflar o binário com ~5MB de JS/CSS de terceiros e
// elimina a necessidade de atualizar assets vendorizados manualmente. A
// contrapartida é exigir acesso à internet no navegador de quem acessa
// /docs/ — aceitável para documentação de desenvolvimento/integração.
func (h *Handler) ServeUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIPage))
}

// swaggerUIPage é a página HTML mínima que inicializa o Swagger UI a partir
// da spec servida em /docs/openapi.json.
const swaggerUIPage = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <title>Streamedia — Documentação da API (Swagger)</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: "/docs/openapi.json",
        dom_id: "#swagger-ui",
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      });
    };
  </script>
</body>
</html>
`
