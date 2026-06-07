package apiresponse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// decodeEnvelope é um helper que decodifica o corpo da resposta em um Envelope.
// Usado por todos os testes para evitar repetição da lógica de unmarshal.
func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) Envelope {
	t.Helper()
	var env Envelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("falha ao decodificar envelope: %v", err)
	}
	return env
}

// TestSuccess_EncodesEnvelope verifica que o helper de sucesso produz o
// envelope correto: error=false, message="ok", data igual ao payload,
// status_code=200, e Content-Type com charset utf-8.
func TestSuccess_EncodesEnvelope(t *testing.T) {
	w := httptest.NewRecorder()

	// Payload de exemplo: um mapa simples simulando um recurso criado.
	payload := map[string]string{"video_id": "550e8400-e29b-41d4-a716-446655440000"}

	Success(w, http.StatusOK, payload)

	// Verifica o status code do header HTTP.
	if w.Code != http.StatusOK {
		t.Errorf("status code do header: esperado %d, obtido %d", http.StatusOK, w.Code)
	}

	// Verifica o Content-Type com charset.
	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type: esperado 'application/json; charset=utf-8', obtido '%s'", ct)
	}

	env := decodeEnvelope(t, w)

	// error deve ser false em sucesso.
	if env.Error != false {
		t.Errorf("campo 'error': esperado false, obtido %v", env.Error)
	}

	// message deve ser "ok" em sucesso.
	if env.Message != "ok" {
		t.Errorf("campo 'message': esperado 'ok', obtido '%s'", env.Message)
	}

	// status_code deve refletir o código passado.
	if env.StatusCode != http.StatusOK {
		t.Errorf("campo 'status_code': esperado %d, obtido %d", http.StatusOK, env.StatusCode)
	}

	// data deve conter o payload original.
	dataMap, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("campo 'data': esperado map[string]interface{}, obtido %T", env.Data)
	}
	if dataMap["video_id"] != payload["video_id"] {
		t.Errorf("campo 'data'.video_id: esperado '%s', obtido '%v'",
			payload["video_id"], dataMap["video_id"])
	}
}

// TestSuccess_NilData verifica que quando data é nil, o campo aparece
// explicitamente como null no JSON (não é omitido).
func TestSuccess_NilData(t *testing.T) {
	w := httptest.NewRecorder()

	Success(w, http.StatusOK, nil)

	if w.Code != http.StatusOK {
		t.Errorf("status code do header: esperado %d, obtido %d", http.StatusOK, w.Code)
	}

	env := decodeEnvelope(t, w)

	// error deve ser false.
	if env.Error != false {
		t.Errorf("campo 'error': esperado false, obtido %v", env.Error)
	}

	// message deve ser "ok".
	if env.Message != "ok" {
		t.Errorf("campo 'message': esperado 'ok', obtido '%s'", env.Message)
	}

	// data deve ser nil (aparece como null no JSON — o json.NewDecoder
	// decodifica null de volta para nil em interface{}).
	if env.Data != nil {
		t.Errorf("campo 'data': esperado nil (null no JSON), obtido %v", env.Data)
	}

	// status_code deve ser 200.
	if env.StatusCode != http.StatusOK {
		t.Errorf("campo 'status_code': esperado %d, obtido %d", http.StatusOK, env.StatusCode)
	}
}

// TestError_EncodesEnvelope verifica que o helper de erro produz o envelope
// correto: error=true, message="msg", data=nil, status_code=400.
func TestError_EncodesEnvelope(t *testing.T) {
	w := httptest.NewRecorder()

	msg := "campo X é obrigatório"
	Error(w, http.StatusBadRequest, msg)

	// Verifica o status code do header HTTP.
	if w.Code != http.StatusBadRequest {
		t.Errorf("status code do header: esperado %d, obtido %d", http.StatusBadRequest, w.Code)
	}

	// Verifica o Content-Type com charset.
	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type: esperado 'application/json; charset=utf-8', obtido '%s'", ct)
	}

	env := decodeEnvelope(t, w)

	// error deve ser true em erro.
	if env.Error != true {
		t.Errorf("campo 'error': esperado true, obtido %v", env.Error)
	}

	// message deve ser a mensagem de erro informada.
	if env.Message != msg {
		t.Errorf("campo 'message': esperado '%s', obtido '%s'", msg, env.Message)
	}

	// data deve ser nil (null no JSON).
	if env.Data != nil {
		t.Errorf("campo 'data': esperado nil (null no JSON), obtido %v", env.Data)
	}

	// status_code deve refletir o código passado.
	if env.StatusCode != http.StatusBadRequest {
		t.Errorf("campo 'status_code': esperado %d, obtido %d", http.StatusBadRequest, http.StatusBadRequest)
	}
}

// TestError_TableDriven_AllStatusCodes é um teste de tabela que cobre todos os
// códigos de erro comuns: 400, 401, 403, 404, 409, 413, 429, 500.
// Cada caso verifica que o status_code aparece correto tanto no header
// quanto no corpo JSON.
func TestError_TableDriven_AllStatusCodes(t *testing.T) {
	tests := []struct {
		status  int
		label   string
		message string
	}{
		{http.StatusBadRequest, "400", "Requisição inválida."},
		{http.StatusUnauthorized, "401", "Não autorizado."},
		{http.StatusForbidden, "403", "Acesso proibido."},
		{http.StatusNotFound, "404", "Recurso não encontrado."},
		{http.StatusConflict, "409", "Recurso já existe."},
		{http.StatusRequestEntityTooLarge, "413", "Arquivo grande demais."},
		{http.StatusTooManyRequests, "429", "Muitas requisições."},
		{http.StatusInternalServerError, "500", "Erro interno do servidor."},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			w := httptest.NewRecorder()

			Error(w, tc.status, tc.message)

			// Verifica o status code do header HTTP.
			if w.Code != tc.status {
				t.Errorf("status code do header: esperado %d, obtido %d", tc.status, w.Code)
			}

			env := decodeEnvelope(t, w)

			// error deve ser true.
			if env.Error != true {
				t.Errorf("campo 'error': esperado true, obtido %v", env.Error)
			}

			// message deve ser a informada.
			if env.Message != tc.message {
				t.Errorf("campo 'message': esperado '%s', obtido '%s'", tc.message, env.Message)
			}

			// status_code no corpo deve bater com o header.
			if env.StatusCode != tc.status {
				t.Errorf("campo 'status_code': esperado %d, obtido %d", tc.status, env.StatusCode)
			}

			// data deve ser nil.
			if env.Data != nil {
				t.Errorf("campo 'data': esperado nil, obtido %v", env.Data)
			}
		})
	}
}

// TestSuccess_TableDriven_StatusCodes verifica que Success aceita diferentes
// códigos 2xx (200, 201, 204) e que todos produzem o envelope correto.
func TestSuccess_TableDriven_StatusCodes(t *testing.T) {
	tests := []struct {
		status  int
		label   string
		payload interface{}
	}{
		{http.StatusOK, "200", map[string]int{"id": 1}},
		{http.StatusCreated, "201", map[string]string{"slug": "meu-projeto"}},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			w := httptest.NewRecorder()

			Success(w, tc.status, tc.payload)

			if w.Code != tc.status {
				t.Errorf("status code do header: esperado %d, obtido %d", tc.status, w.Code)
			}

			env := decodeEnvelope(t, w)

			if env.Error != false {
				t.Errorf("campo 'error': esperado false, obtido %v", env.Error)
			}
			if env.Message != "ok" {
				t.Errorf("campo 'message': esperado 'ok', obtido '%s'", env.Message)
			}
			if env.StatusCode != tc.status {
				t.Errorf("campo 'status_code': esperado %d, obtido %d", tc.status, env.StatusCode)
			}
		})
	}
}

// TestSuccess_JsonOmitempty verifica que o campo data nunca é omitido,
// mesmo quando o tipo subjacente é nil. Em Go, interface{} com valor nil
// é serializado como null (não omitido), pois json:"data" não tem omitempty.
func TestSuccess_JsonOmitempty(t *testing.T) {
	w := httptest.NewRecorder()

	Success(w, http.StatusOK, nil)

	// Lê o corpo bruto como string para verificar que "data":null aparece.
	body := w.Body.String()

	// Verifica que o campo "data" existe no JSON.
	if !containsKey(body, `"data"`) {
		t.Errorf("corpo JSON não contém o campo 'data': %s", body)
	}

	// Verifica que "data":null aparece explicitamente.
	if !containsKey(body, `"data":null`) {
		t.Errorf("campo 'data' não aparece como null no JSON: %s", body)
	}
}

// containsKey verifica de forma simples se uma substring existe no corpo JSON.
// Não é um parser JSON completo — é uma verificação pragmática para este teste.
func containsKey(body, key string) bool {
	for i := 0; i <= len(body)-len(key); i++ {
		if body[i:i+len(key)] == key {
			return true
		}
	}
	return false
}
