// Pacote admin — handlers de gerenciamento de configurações dinâmicas.
//
// As configurações ficam na tabela configurations (ver migration 0004).
// Cada configuração tem: key, value, type, description, group_key, validation, visible.
//
// Configurações com visible=0 (tipo "secret") NUNCA têm seu valor retornado
// no GET — são write-only: só aceitam atualização via PUT.
//
// As configurações são agrupadas por group_key para exibição na UI. O formato
// de resposta é gerado por models.BuildConfigGroups, que combina dados do
// banco com defaults do código Go.
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/models"
)

// HandleGetConfig retorna todas as configurações agrupadas por categoria.
// Configurações com visible=false têm o campo "value" omitido.
//
// GET /admin/config
func (h *AdminHandler) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	groups, err := models.BuildConfigGroups(h.db)
	if err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao carregar configurações.")
		return
	}
	apiresponse.Success(w, http.StatusOK, groups)
}

// HandleUpdateConfig atualiza o valor de UMA configuração específica.
// A chave vem do path (chi URL param), o valor do body JSON.
//
// PUT /admin/config/{key}
// Body: { "value": "novo_valor" }
//
// Valida o valor conforme o tipo e a regex da configuração antes de persistir.
// Para configs do tipo "secret" (visible=false), o valor antigo nunca foi
// exposto — a atualização é sempre "cega" (escreve por cima).
func (h *AdminHandler) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		apiresponse.Error(w, http.StatusBadRequest, "Chave de configuração é obrigatória.")
		return
	}

	// Lê o body.
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, "Body inválido: informe um JSON com o campo 'value'.")
		return
	}

	// Busca a configuração para validar o tipo e a regex.
	cfg, err := models.GetConfiguration(h.db, key)
	if err != nil {
		// Se a configuração não existe no banco, usa os defaults para validação.
		// Cria a entrada usando UpsertConfiguration (INSERT OR REPLACE via ON CONFLICT).
		if _, known := models.DefaultValues[key]; !known {
			apiresponse.Error(w, http.StatusNotFound, "Configuração desconhecida.")
			return
		}

		// Validação básica: persiste o valor. A validação de tipo mais rigorosa
		// depende dos metadados no banco (type, validation), que só existem
		// quando a linha já foi inserida pela migration.
		if err := models.UpsertConfiguration(h.db, key, body.Value); err != nil {
			apiresponse.Error(w, http.StatusInternalServerError, "Erro ao salvar configuração.")
			return
		}
		h.cfg.ReloadFromDB(h.db, key)
		apiresponse.Success(w, http.StatusOK, map[string]string{"key": key, "value": body.Value})
		return
	}

	// Valida o valor conforme o tipo e a regex da configuração.
	if err := models.ValidateConfigValue(cfg.Type, cfg.Validation, body.Value); err != nil {
		apiresponse.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Persiste.
	if err := models.UpsertConfiguration(h.db, key, body.Value); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao salvar configuração.")
		return
	}

	// Aplica a mudança em tempo real no Config em memória (sem reiniciar).
	h.cfg.ReloadFromDB(h.db, key)

	apiresponse.Success(w, http.StatusOK, map[string]string{"key": key, "value": body.Value})
}

// HandleDeleteConfig remove UMA configuração do banco. Após a remoção, o
// sistema passa a usar o DefaultValues[key] como fallback.
// Só pode ser executado por usuários com role "dev".
//
// DELETE /admin/config/{key}
func (h *AdminHandler) HandleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		apiresponse.Error(w, http.StatusBadRequest, "Chave de configuração é obrigatória.")
		return
	}

	// Verifica se é uma configuração conhecida.
	if _, known := models.DefaultValues[key]; !known {
		apiresponse.Error(w, http.StatusNotFound, "Configuração desconhecida.")
		return
	}

	if err := models.DeleteConfiguration(h.db, key); err != nil {
		apiresponse.Error(w, http.StatusInternalServerError, "Erro ao remover configuração.")
		return
	}

	// Recarrega do banco (vai usar o DefaultValues como fallback).
	h.cfg.ReloadFromDB(h.db, key)

	apiresponse.Success(w, http.StatusOK, map[string]string{"key": key, "deleted": "true"})
}
