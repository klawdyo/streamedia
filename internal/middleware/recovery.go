// Package middleware contém middlewares HTTP reutilizáveis da aplicação:
// rate limiting por IP e recuperação de panics no formato padrão da API.
package middleware

import (
	"log"
	"net/http"

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
