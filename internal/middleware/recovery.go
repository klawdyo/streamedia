// Package middleware contém middlewares HTTP reutilizáveis da aplicação:
// rate limiting por IP, recuperação de panics e normalização de trailing slash.
package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/klawdyo/streamedia/internal/apiresponse"
)

// RecoveryMiddleware recupera de panics em qualquer handler da cadeia,
// loga o erro internamente e responde ao cliente com o envelope padrão
// de erro (status 500, mensagem genérica em português).
//
// Substitui chimw.Recoverer: a versão do chi responde com texto puro,
// quebrando o contrato JSON da API. Este middleware garante que até
// mesmo panics não tratados sigam o formato {error, message, data, status_code}.
//
// A mensagem devolvida ao cliente é sempre genérica ("Erro interno
// desconhecido.") — o conteúdo original do panic NUNCA é exposto ao
// cliente, prevenindo vazamento de detalhes internos (stack traces,
// paths de arquivo, mensagens de bibliotecas, etc.). O panic original
// é logado via log.Printf para debugging interno.
//
// Após recuperar um panic, o handler seguinte na cadeia continua
// funcionando normalmente — o servidor não é derrubado.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// defer captura qualquer panic que escape do handler abaixo.
		defer func() {
			if rec := recover(); rec != nil {
				// Loga o panic original para debugging interno. A mensagem
				// inclui o método e o path da requisição para facilitar
				// a correlação nos logs.
				log.Printf("[panic] recuperado: %v — requisição %s %s",
					rec, r.Method, r.URL.Path)

				// Responde ao cliente com o envelope padrão de erro.
				// Mensagem genérica — nunca vaza o conteúdo do panic.
				apiresponse.Error(w, http.StatusInternalServerError, "Erro interno desconhecido.")
			}
		}()

		// Encadeia para o próximo handler na pilha de middlewares.
		next.ServeHTTP(w, r)
	})
}

// StripSlashMiddleware normaliza todas as requisições removendo a barra
// final do path (ex.: /docs/ → /docs) antes do roteamento. Isso permite
// registrar cada rota uma única vez sem se preocupar com trailing slash
// — o chi recebe o path já normalizado e casa corretamente.
//
// Diferente de chimw.RedirectSlashes, NÃO faz redirect (301) — apenas
// reescreve o path internamente. O cliente nunca vê diferença na URL,
// mas a rota casa independentemente da barra final.
//
// Exceção: o path raiz "/" não é alterado (remover a barra resultaria
// em string vazia, quebrando o roteamento).
func StripSlashMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if len(path) > 1 && strings.HasSuffix(path, "/") {
			r.URL.Path = strings.TrimRight(path, "/")
		}
		next.ServeHTTP(w, r)
	})
}
