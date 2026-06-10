// Pacote testui expõe uma interface web interativa (single-file, sem build
// step) que exercita o pipeline completo do Streamedia de ponta a ponta:
// autenticação (ROOT_TOKEN) → upload/init → upload TUS em chunks →
// play/init → reprodução HLS por resolução. (issue #18)
//
// Além da página em GET /test, o pacote oferece um receptor de webhooks de
// teste: POST /test/webhook recebe os webhooks enviados pelo próprio
// Streamedia e os mantém num buffer em memória, que a página consulta via
// GET /test/webhook/events para exibi-los ao vivo. Para que os webhooks
// cheguem aqui, basta apontar a variável de ambiente WEBHOOK_URL para
// <origin>/test/webhook — a página mostra essa URL pronta para copiar.
//
// Decisão de escopo: o receptor é puramente local e em memória (não há
// override de webhook por requisição em /api/upload/init, nem persistência).
// Isso mantém a ferramenta autocontida e sem tocar no fluxo de produção —
// coerente com o "Fora de escopo" da issue (sem persistência de estado).
package testui

import (
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// indexHTML é a página de teste, embutida no binário em tempo de build.
// Mantida como arquivo .html separado (e não string Go) para preservar o
// destaque de sintaxe do editor e a legibilidade — segue "single-file,
// sem build step" do ponto de vista de quem usa: o navegador recebe um
// único HTML autocontido, sem bundler.
//
//go:embed index.html
var indexFS embed.FS

// maxWebhookEvents limita o buffer em memória de webhooks recebidos. Como a
// ferramenta é de teste/demonstração e não persiste estado, guardamos apenas
// os mais recentes; ao exceder o limite, os mais antigos são descartados.
const maxWebhookEvents = 50

// webhookEvent representa um webhook recebido em POST /test/webhook, já
// decomposto em metadados úteis para exibição na página.
type webhookEvent struct {
	// Seq é um contador monotônico crescente; a página o usa para buscar
	// apenas os eventos novos (polling incremental via ?since=).
	Seq        int               `json:"seq"`
	ReceivedAt time.Time         `json:"received_at"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	// RawBody é o corpo cru recebido (string). A página tenta formatá-lo como
	// JSON; se não for JSON válido, exibe como texto.
	RawBody string `json:"raw_body"`
}

// Handler agrega o estado do receptor de webhooks (buffer em memória
// protegido por mutex) e serve tanto a página quanto os endpoints auxiliares.
type Handler struct {
	mu     sync.Mutex
	events []webhookEvent
	seq    int
}

// NewHandler cria um Handler de teste com o buffer de webhooks vazio.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeUI devolve a página HTML de teste (GET /test).
func (h *Handler) ServeUI(w http.ResponseWriter, _ *http.Request) {
	page, err := indexFS.ReadFile("index.html")
	if err != nil {
		// O arquivo é embutido em build; um erro aqui indicaria um problema de
		// empacotamento, não uma condição de runtime.
		http.Error(w, "página de teste indisponível", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

// ReceiveWebhook recebe um webhook enviado pelo Streamedia (POST /test/webhook),
// captura seus cabeçalhos e corpo e os guarda no buffer em memória. Responde
// 200 com um corpo JSON simples — o cliente de webhook do Streamedia considera
// qualquer 2xx como sucesso, então não há retry desnecessário.
func (h *Handler) ReceiveWebhook(w http.ResponseWriter, r *http.Request) {
	// Lê o corpo com limite de 1MB — payloads de webhook são pequenos.
	bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	// Captura todos os cabeçalhos (primeiro valor de cada um) para exibição.
	headers := make(map[string]string, len(r.Header))
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	h.mu.Lock()
	h.seq++
	event := webhookEvent{
		Seq:        h.seq,
		ReceivedAt: time.Now().UTC(),
		Method:     r.Method,
		Headers:    headers,
		RawBody:    string(bodyBytes),
	}
	h.events = append(h.events, event)
	// Mantém apenas os mais recentes (descarta os mais antigos do início).
	if len(h.events) > maxWebhookEvents {
		h.events = h.events[len(h.events)-maxWebhookEvents:]
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"received":true}`))
}

// ListEvents devolve, em JSON, os webhooks recebidos desde um determinado
// número de sequência (GET /test/webhook/events?since=N). A página faz polling
// neste endpoint e passa o maior Seq já visto para receber apenas os novos.
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	since := 0
	if s := r.URL.Query().Get("since"); s != "" {
		// Ignora valores inválidos (since permanece 0 → devolve tudo).
		_ = json.Unmarshal([]byte(s), &since)
	}

	h.mu.Lock()
	out := make([]webhookEvent, 0, len(h.events))
	for _, e := range h.events {
		if e.Seq > since {
			out = append(out, e)
		}
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
