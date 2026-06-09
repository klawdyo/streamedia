// Pacote docs expõe a documentação interativa da API HTTP do Streamedia
// (issue #3 / T30, revisado pela issue #12): uma especificação OpenAPI
// servida em JSON e uma UI que a consome, ambas acessíveis via navegador
// em /docs/.
//
// Decisão de UI (issue #12): o autor do projeto considerou a UI padrão do
// Swagger pouco agradável visualmente e pediu alternativas. Trocamos o
// Swagger UI (entregue originalmente no T30) pelo Scalar
// (https://scalar.com/), que consome a mesma especificação OpenAPI sem
// nenhuma mudança de contrato — apenas a página HTML servida em /docs/ foi
// alterada.
//
// Decisão de autenticação (revisada — endurecimento de segurança): a
// documentação era SERVIDA SEM autenticação, mas passou a exigir token de
// admin (AdminAuth), no mesmo grupo da rota /metrics. Motivo: descrever toda
// a superfície da API é informação valiosa para reconhecimento por
// scanners/bots que varrem IPs públicos. A montagem do middleware é feita no
// roteador (internal/server/server.go), não aqui — este handler só serve o
// conteúdo. O rate limiter global (T19) continua mitigando abuso.
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

// ServeUI devolve uma página HTML que carrega o Scalar (via CDN) apontando
// para a spec servida em /docs/openapi.json (GET /docs/ ou /docs).
//
// Optamos por carregar o componente do Scalar via CDN (jsDelivr) em vez de
// embuti-lo no binário: o impacto no tamanho do executável fica desprezível
// (a página é só um HTML estático de poucas centenas de bytes), e evita
// vendorizar/atualizar manualmente um bundle JS de terceiros. A
// contrapartida é exigir acesso à internet no navegador de quem acessa
// /docs/ — aceitável para documentação de desenvolvimento/integração.
func (h *Handler) ServeUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(scalarUIPage))
}

// scalarUIPage é a página HTML mínima que inicializa o Scalar a partir da
// spec servida em /docs/openapi.json.
const scalarUIPage = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <title>Streamedia — Documentação da API (Scalar)</title>
</head>
<body>
  <script id="api-reference" data-url="/docs/openapi.json"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>
`
