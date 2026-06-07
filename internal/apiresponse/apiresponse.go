// Package apiresponse define o envelope padrão de toda resposta JSON da API.
//
// Unifica o que antes eram 3+ implementações divergentes de respondError
// espalhadas pelos pacotes — qualquer rota nova OU qualquer mudança no
// formato passa a acontecer em um único lugar.
//
// O envelope segue o formato:
//
//	{
//	  "error": false,
//	  "message": "ok",
//	  "data": { },
//	  "status_code": 200
//	}
//
// Regras:
//   - error (bool): false em sucesso, true em qualquer erro
//   - message (string): mensagem legível; em sucesso, "ok"; em erro, a
//     mensagem descritiva do problema (em português, conforme convenção do
//     projeto para mensagens de erro da API)
//   - data (object | array | null): o payload em sucesso; null em erro
//   - status_code (int): o código de status HTTP, repetido também no header
//     da resposta para consumidores que não parseiam o corpo
//
// Content-Type de toda resposta: "application/json; charset=utf-8"
package apiresponse

import (
	"encoding/json"
	"net/http"
)

// Envelope é o formato padrão de toda resposta JSON da API.
// Toda rota — de sucesso ou erro — deve usar as funções Success e Error
// deste pacote em vez de escrever JSON manualmente.
type Envelope struct {
	Error      bool        `json:"error"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data"`
	StatusCode int         `json:"status_code"`
}

// Success escreve uma resposta de sucesso no envelope padrão.
//
// Campos preenchidos automaticamente:
//   - error: false
//   - message: "ok"
//   - data: o payload informado (pode ser nil — aparece como null no JSON)
//   - status_code: o código HTTP informado (use 200 para o caso comum;
//     aceita outros códigos 2xx, ex. 201 em criação de recurso)
//
// O Content-Type é sempre "application/json; charset=utf-8".
func Success(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Error:      false,
		Message:    "ok",
		Data:       data,
		StatusCode: status,
	})
}

// Error escreve uma resposta de erro no envelope padrão.
//
// Campos preenchidos automaticamente:
//   - error: true
//   - message: a mensagem descritiva do problema (em português, conforme
//     convenção do projeto)
//   - data: null (sempre — o campo aparece explicitamente no JSON)
//   - status_code: o código HTTP informado (400, 401, 403, 404, 409, 413,
//     429, 500, etc.)
//
// O Content-Type é sempre "application/json; charset=utf-8".
// O status code é repetido tanto no header HTTP quanto no corpo JSON,
// para facilitar o consumo por clientes que não inspecionam headers.
func Error(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{
		Error:      true,
		Message:    msg,
		Data:       nil,
		StatusCode: status,
	})
}
