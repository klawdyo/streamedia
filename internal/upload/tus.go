// Pacote upload implementa o protocolo TUS de upload resumível e a rota de inicialização.
package upload

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"

	"github.com/klawdyo/streamedia/internal/apiresponse"
	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// ctxKeyVideoID é a chave de contexto usada para propagar o video_id extraído
// da URL (/files/{videoID}) desde o ServeHTTP até o hook preCreate.
//
// É necessária porque o roteador embutido do tusd/v2 só aceita POST de criação
// no BasePath raiz (/files/) — um POST para /files/{videoID} resultaria em 405.
// Por isso, no ServeHTTP reescrevemos o path do POST para o raiz e guardamos o
// video_id no contexto, de onde o preCreate o recupera para fixar o ID do upload.
type ctxKeyVideoID struct{}

// jsonContentType é o cabeçalho Content-Type usado nas respostas de erro
// do tusd. Em tusd/v2 o HTTPResponse.Header é um map[string]string
// (tusd.HTTPHeader), e não um http.Header. Inclui charset utf-8 para
// consistência com o pacote apiresponse.
var jsonContentType = tusd.HTTPHeader{"Content-Type": "application/json; charset=utf-8"}

// tusErrorBody serializa um apiresponse.Envelope de erro como string JSON
// para uso em tusd.HTTPResponse.Body. O tusd gerencia a escrita HTTP
// internamente, então não podemos usar apiresponse.Error (que escreve
// diretamente no http.ResponseWriter). Para manter o contrato de envelope
// uniforme, serializamos manualmente o mesmo formato aqui.
func tusErrorBody(status int, msg string) string {
	b, _ := json.Marshal(apiresponse.Envelope{
		Error:      true,
		Message:    msg,
		Data:       nil,
		StatusCode: status,
	})
	return string(b)
}

// TUSHandler encapsula o handler TUS do tusd com autenticação e hooks personalizados.
type TUSHandler struct {
	handler  *tusd.Handler
	cfg      *config.Config
	db       *sql.DB
	onFinish func(videoID, userAgent string)
}

// NewTUSHandler cria um novo TUSHandler com validação de token e hooks de ciclo de vida.
// O parâmetro onFinish é chamado quando o upload completa (para validação e enfileiramento).
func NewTUSHandler(cfg *config.Config, db *sql.DB, onFinish func(videoID, userAgent string)) (*TUSHandler, error) {
	// Cria o FileStore — armazena os chunks no diretório de uploads temporários.
	store := filestore.New(cfg.UploadTmpDir)

	// Registra o store no Composer do tusd.
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)

	h := &TUSHandler{
		cfg:      cfg,
		db:       db,
		onFinish: onFinish,
	}

	// Configuração do handler tusd.
	//
	// Diferença importante em relação à API antiga: no tusd/v2 os hooks de
	// "após receber chunk" e "após finalizar" NÃO são callbacks na Config.
	// Eles são entregues por meio de canais no próprio Handler
	// (UploadProgress e CompleteUploads), habilitados pelas flags
	// NotifyUploadProgress e NotifyCompleteUploads. Por isso processamos
	// esses eventos em goroutines que consomem os canais (ver consumeHooks).
	//
	// Já o hook de pré-criação continua sendo um callback síncrono na Config:
	// PreUploadCreateCallback.
	tusConfig := tusd.Config{
		BasePath:      "/files/",
		StoreComposer: composer,
		MaxSize:       cfg.MaxUploadSizeBytes,
		// Hook chamado antes de criar o upload — valida token e tamanho.
		PreUploadCreateCallback: h.preCreate,
		// Habilita o envio de notificações pelos canais do handler.
		NotifyUploadProgress:  true,
		NotifyCompleteUploads: true,
	}

	handler, err := tusd.NewHandler(tusConfig)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar handler TUS: %w", err)
	}

	h.handler = handler

	// Consome os canais de notificação em background.
	go h.consumeHooks()

	return h, nil
}

// consumeHooks lê continuamente os canais de notificação do tusd e despacha
// para os tratadores postReceive (progresso de chunk) e postFinish (upload
// completo). Substitui os callbacks PostReceiveCallback/PostFinishCallback que
// não existem no tusd/v2.
func (h *TUSHandler) consumeHooks() {
	for {
		select {
		case event := <-h.handler.UploadProgress:
			h.postReceive(event)
		case event := <-h.handler.CompleteUploads:
			h.postFinish(event)
		}
	}
}

// ServeHTTP valida o token de upload ANTES de delegar ao tusd.
// A validação aqui (e não só no preCreate hook) garante que 401 é retornado
// mesmo quando o tusd rejeitaria com 405 por questões de protocolo/método.
func (h *TUSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extrai e valida o token para qualquer método TUS relevante.
	token := r.Header.Get("Upload-Token")
	if token == "" {
		apiresponse.Error(w, http.StatusUnauthorized, "Token de upload ausente.")
		return
	}

	// Extrai o video_id do path da requisição.
	videoID := extractVideoIDFromPath(r.URL.Path)

	// Busca e valida o token no banco.
	uploadToken, err := models.GetUploadToken(h.db, token)
	if err != nil || uploadToken.IsExpired() {
		apiresponse.Error(w, http.StatusUnauthorized, "Token de upload inválido ou expirado.")
		return
	}

	// Garante que o token pertence ao video_id da URL (proteção contra reutilização).
	if videoID != "" && uploadToken.VideoID != videoID {
		apiresponse.Error(w, http.StatusForbidden, "Token de upload não corresponde ao vídeo informado.")
		return
	}

	// Verifica o tamanho declarado no header Upload-Length.
	if lengthStr := r.Header.Get("Upload-Length"); lengthStr != "" {
		if length, err := strconv.ParseInt(lengthStr, 10, 64); err == nil {
			if length > h.cfg.MaxUploadSizeBytes {
				apiresponse.Error(w, http.StatusRequestEntityTooLarge, "O vídeo excede o tamanho máximo permitido.")
				return
			}
		}
	}

	// O roteador interno do tusd/v2 avalia o path RELATIVO ao ponto de montagem:
	// ele compara strings.Trim(path, "/") com "" para o endpoint de criação (POST)
	// e trata qualquer outro path como um recurso de upload (HEAD/PATCH/...). Como
	// o nosso ServeHTTP já é o ponto de montagem em /files/, removemos esse prefixo
	// antes de delegar — caso contrário o tusd responderia 405.
	r = stripFilesPrefix(r)

	// No POST de criação, o tusd só aceita o path raiz (vazio). Por isso forçamos
	// o path para "/" e propagamos o video_id no contexto, de onde o preCreate o
	// recupera para fixar o ID do upload (que precisa ser o video_id).
	if r.Method == http.MethodPost && videoID != "" {
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyVideoID{}, videoID))
		r.URL.Path = "/"
	}

	// Autenticação OK — delega para o handler interno do tusd.
	h.handler.ServeHTTP(w, r)
}

// stripFilesPrefix devolve uma cópia da requisição com o prefixo /files removido
// do path, de modo que o roteador do tusd veja o path relativo ao ponto de montagem.
func stripFilesPrefix(r *http.Request) *http.Request {
	trimmed := strings.TrimPrefix(r.URL.Path, "/files")
	if trimmed == "" {
		trimmed = "/"
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = trimmed
	return r2
}

// preCreate é chamado pelo tusd antes de criar um novo upload.
// Valida o token de upload e verifica o tamanho declarado.
func (h *TUSHandler) preCreate(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
	// Recupera o video_id propagado pelo ServeHTTP via contexto. Em fallback,
	// extrai do path da URI (caso a requisição não tenha passado pelo nosso
	// ServeHTTP, p.ex. em testes diretos do hook).
	videoID, _ := hook.Context.Value(ctxKeyVideoID{}).(string)
	if videoID == "" {
		videoID = extractVideoIDFromPath(hook.HTTPRequest.URI)
	}

	// Extrai o token do header Upload-Token.
	token := hook.HTTPRequest.Header.Get("Upload-Token")
	if token == "" {
		return tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       tusErrorBody(http.StatusUnauthorized, "Token de upload ausente."),
			Header:     jsonContentType,
		}, tusd.FileInfoChanges{}, nil
	}

	// Busca o token no banco e valida.
	uploadToken, err := models.GetUploadToken(h.db, token)
	if err != nil {
		return tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       tusErrorBody(http.StatusUnauthorized, "Token de upload inválido ou não encontrado."),
			Header:     jsonContentType,
		}, tusd.FileInfoChanges{}, nil
	}

	// Verifica se o token está expirado.
	if uploadToken.IsExpired() {
		return tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       tusErrorBody(http.StatusUnauthorized, "Token de upload expirado."),
			Header:     jsonContentType,
		}, tusd.FileInfoChanges{}, nil
	}

	// Verifica que o token pertence ao video_id da URL.
	if videoID != "" && uploadToken.VideoID != videoID {
		return tusd.HTTPResponse{
			StatusCode: http.StatusForbidden,
			Body:       tusErrorBody(http.StatusForbidden, "Token de upload não corresponde ao vídeo informado."),
			Header:     jsonContentType,
		}, tusd.FileInfoChanges{}, nil
	}

	// Verifica o tamanho declarado (Upload-Length header).
	uploadLengthStr := hook.HTTPRequest.Header.Get("Upload-Length")
	if uploadLengthStr != "" {
		uploadLength, errLen := strconv.ParseInt(uploadLengthStr, 10, 64)
		if errLen == nil && uploadLength > h.cfg.MaxUploadSizeBytes {
			return tusd.HTTPResponse{
				StatusCode: http.StatusRequestEntityTooLarge,
				Body:       tusErrorBody(http.StatusRequestEntityTooLarge, "O vídeo excede o tamanho máximo permitido."),
				Header:     jsonContentType,
			}, tusd.FileInfoChanges{}, nil
		}
	}

	// Força que o ID do upload no tusd seja o video_id da URL, para que os
	// hooks de progresso/finalização possam correlacionar com o banco.
	changes := tusd.FileInfoChanges{}
	if videoID != "" {
		changes.ID = videoID
	}

	return tusd.HTTPResponse{}, changes, nil
}

// postReceive é chamado após cada chunk recebido.
// Atualiza o last_chunk_at e o status do vídeo para 'uploading'.
func (h *TUSHandler) postReceive(hook tusd.HookEvent) {
	// O ID do upload no tusd corresponde ao video_id.
	videoID := hook.Upload.ID

	// Atualiza o timestamp do último chunk recebido.
	if err := models.UpdateLastChunk(h.db, videoID); err != nil {
		// Falha não-crítica: apenas ignora (o upload continua).
		_ = err
	}

	// Atualiza status para 'uploading' se ainda estiver em pending_upload.
	video, err := models.GetVideo(h.db, videoID)
	if err == nil && video.Status == models.StatusPendingUpload {
		_ = models.UpdateStatus(h.db, videoID, models.StatusUploading)
	}
}

// postFinish é chamado quando o upload completa com todos os chunks.
// Delega para o callback onFinish que fará validação e enfileiramento.
func (h *TUSHandler) postFinish(hook tusd.HookEvent) {
	videoID := hook.Upload.ID
	if h.onFinish != nil {
		// Repassa o User-Agent da requisição de finalização para que o evento
		// de estatística "upload_complete" (T26/T27) seja registrado com a
		// família de SO correta.
		h.onFinish(videoID, hook.HTTPRequest.Header.Get("User-Agent"))
	}
}

// extractVideoIDFromPath extrai o video_id do path TUS (/files/{videoID}).
func extractVideoIDFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/files/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}
