// Pacote spa serve a Single Page Application (Vue.js) do admin unificado
// a partir do diretório de build de produção (web/dist/).
//
// Em desenvolvimento, o Vite roda separado com proxy reverso — este pacote
// não é usado. Em produção, o Dockerfile copia web/dist/ para dentro da
// imagem e este handler serve os arquivos estáticos.
//
// O diretório é configurável via env var SPA_DIR (default: ./web/dist).
// Se o diretório não existir ou estiver vazio, as rotas /app/* retornam
// 503 Service Unavailable com uma mensagem informativa.
package spa

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Handler serve a SPA e seus assets estáticos.
type Handler struct {
	dir string // caminho absoluto para web/dist/
}

// NewHandler cria um Handler para a SPA. dir é o caminho para o diretório
// de build (ex: /app/web/dist). Se dir for vazio, usa o default "./web/dist".
func NewHandler(dir string) *Handler {
	if dir == "" {
		dir = "./web/dist"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return &Handler{dir: abs}
}

// ServeIndex serve o index.html da SPA. Usado como fallback para qualquer
// rota /app/* que não seja um arquivo estático (SPA routing).
func (h *Handler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	indexPath := filepath.Join(h.dir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		http.Error(w, "Admin UI não disponível — build do frontend não encontrado. Execute 'cd web && npm run build' ou configure SPA_DIR.", http.StatusServiceUnavailable)
		return
	}
	http.ServeFile(w, r, indexPath)
}

// ServeAssets serve os arquivos estáticos da SPA (JS, CSS, imagens, fontes).
// Protege contra path traversal: só serve arquivos DENTRO do diretório SPA.
// Mapeia /app/assets/* → <spa_dir>/assets/*
func (h *Handler) ServeAssets(w http.ResponseWriter, r *http.Request) {
	// Extrai o caminho relativo após /app/
	urlPath := strings.TrimPrefix(r.URL.Path, "/app/")
	if urlPath == "" || urlPath == "/" {
		h.ServeIndex(w, r)
		return
	}

	// Limpa o caminho para prevenir path traversal (ex: /app/../../../etc/passwd).
	cleanPath := filepath.Clean(urlPath)

	// Resolve o caminho absoluto no disco.
	fullPath := filepath.Join(h.dir, cleanPath)

	// Garante que o arquivo está DENTRO do diretório SPA (defesa em profundidade).
	absDir, _ := filepath.Abs(h.dir)
	absFull, err := filepath.Abs(fullPath)
	if err != nil || !strings.HasPrefix(absFull, absDir) {
		http.Error(w, "Acesso negado", http.StatusForbidden)
		return
	}

	// Verifica se o arquivo existe.
	info, err := os.Stat(fullPath)
	if err != nil {
		// Se o arquivo não existe, serve index.html (SPA fallback para
		// rotas client-side como /app/videos, /app/overview, etc.).
		if os.IsNotExist(err) {
			h.ServeIndex(w, r)
			return
		}
		http.Error(w, "Erro ao acessar arquivo", http.StatusInternalServerError)
		return
	}

	// Não permite listar diretórios.
	if info.IsDir() {
		h.ServeIndex(w, r)
		return
	}

	http.ServeFile(w, r, fullPath)
}

// Dir retorna o caminho do diretório SPA configurado.
func (h *Handler) Dir() string {
	return h.dir
}
