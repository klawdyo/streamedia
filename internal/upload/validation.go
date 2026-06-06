// Pacote upload trata o recebimento, validação e finalização de uploads de vídeo.
package upload

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/klawdyo/streamedia/internal/config"
	"github.com/klawdyo/streamedia/internal/models"
)

// validateMagicBytes abre o arquivo, lê os primeiros 12 bytes e verifica se
// correspondem a alguma assinatura conhecida de contêiner de vídeo.
//
// Assinaturas suportadas:
//   - MP4/MOV (caixa ftyp): 66 74 79 70 no offset 4
//   - MKV/WebM (Matroska):  1a 45 df a3 no offset 0
//   - AVI (RIFF):           52 49 46 46 no offset 0
//   - QuickTime (moov):     6d 6f 6f 76 no offset 4
//
// Retorna (false, nil) para arquivos com menos de 8 bytes, vazios ou sem
// assinatura correspondente. Não retorna erro para arquivos vazios.
func validateMagicBytes(path string) (bool, error) {
	// Abre o arquivo somente para leitura
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Lê no máximo 12 bytes do início do arquivo
	buf := make([]byte, 12)
	n, err := f.Read(buf)
	// Arquivo vazio (n == 0) não é um erro: apenas não é vídeo
	if err != nil && n == 0 {
		// EOF em arquivo vazio é esperado; retorna sem erro
		return false, nil
	}

	// É necessário ao menos 8 bytes para checar as assinaturas no offset 4
	if n < 8 {
		return false, nil
	}

	// Assinaturas no offset 0
	// MKV/WebM: 1a 45 df a3
	if buf[0] == 0x1a && buf[1] == 0x45 && buf[2] == 0xdf && buf[3] == 0xa3 {
		return true, nil
	}
	// AVI (RIFF): 52 49 46 46
	if buf[0] == 0x52 && buf[1] == 0x49 && buf[2] == 0x46 && buf[3] == 0x46 {
		return true, nil
	}

	// Assinaturas no offset 4
	// MP4/MOV (ftyp): 66 74 79 70
	if buf[4] == 0x66 && buf[5] == 0x74 && buf[6] == 0x79 && buf[7] == 0x70 {
		return true, nil
	}
	// QuickTime (moov): 6d 6f 6f 76
	if buf[4] == 0x6d && buf[5] == 0x6f && buf[6] == 0x6f && buf[7] == 0x76 {
		return true, nil
	}

	// Nenhuma assinatura reconhecida
	return false, nil
}

// validateFileSize compara o tamanho real do arquivo com o tamanho declarado
// pelo cliente. Retorna nil quando são iguais e um erro descritivo caso difiram.
func validateFileSize(actualBytes, declaredBytes int64) error {
	if actualBytes == declaredBytes {
		return nil
	}
	return fmt.Errorf(
		"tamanho real do arquivo (%d bytes) difere do declarado (%d bytes)",
		actualBytes, declaredBytes,
	)
}

// FFprobeResult contém os metadados extraídos do stream de vídeo via ffprobe.
type FFprobeResult struct {
	DurationS int
	Width     int
	Height    int
}

// ffprobeOutput espelha a estrutura do JSON retornado por ffprobe -show_streams.
type ffprobeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"`
	} `json:"streams"`
}

// runFFprobe executa o ffprobe sobre o arquivo informado e extrai duração,
// largura e altura do primeiro stream de vídeo.
//
// O contexto é usado como recebido (o chamador é responsável por aplicar timeout).
func runFFprobe(ctx context.Context, path string) (*FFprobeResult, error) {
	// Monta e executa o comando ffprobe pedindo saída JSON com os streams
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Faz o parse do JSON retornado
	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}

	// Procura o primeiro stream de vídeo
	for _, s := range parsed.Streams {
		if s.CodecType != "video" {
			continue
		}

		// A duração vem como string (ex.: "3.000000"); converte para float e trunca
		var durationS int
		if s.Duration != "" {
			d, err := strconv.ParseFloat(s.Duration, 64)
			if err != nil {
				return nil, err
			}
			durationS = int(d)
		}

		return &FFprobeResult{
			DurationS: durationS,
			Width:     s.Width,
			Height:    s.Height,
		}, nil
	}

	// Nenhum stream de vídeo encontrado
	return nil, fmt.Errorf("nenhum stream de vídeo encontrado no arquivo")
}

// HandlePostFinish orquestra a validação final de um upload concluído.
//
// Fluxo:
//  1. Busca o vídeo no banco para obter o tamanho declarado.
//  2. Obtém o tamanho real do arquivo em disco.
//  3. Valida o tamanho (real == declarado).
//  4. Valida os magic bytes do contêiner de vídeo.
//  5. Executa ffprobe (timeout de 5s) para confirmar que é um vídeo válido.
//
// Em qualquer falha: marca o status como failed_upload, remove o arquivo e o
// seu .info, e dispara o webhook "failed". Em caso de sucesso: marca como
// upload_complete, enfileira a transcodificação e dispara o webhook "processing".
func HandlePostFinish(
	db *sql.DB,
	cfg *config.Config,
	enqueue func(videoID string) error,
	sendWebhook func(videoID string, event string, errMsg string),
	videoID string,
	filePath string,
) {
	// fail centraliza o tratamento de falha: persiste o erro, remove arquivos
	// e notifica via webhook.
	fail := func(errMsg string) {
		// Marca o vídeo como falho gravando a mensagem de erro
		_ = models.UpdateStatusWithError(db, videoID, models.StatusFailedUpload, errMsg)
		// Remove o arquivo de vídeo e o arquivo de metadados associado
		_ = os.Remove(filePath)
		_ = os.Remove(filePath + ".info")
		// Notifica o cliente sobre a falha
		sendWebhook(videoID, "failed", errMsg)
	}

	// 1. Busca o vídeo no banco (pode não existir ainda)
	video, err := models.GetVideo(db, videoID)
	if err != nil {
		fail(fmt.Sprintf("vídeo não encontrado no banco: %v", err))
		return
	}

	// 2. Obtém o tamanho real do arquivo em disco
	info, err := os.Stat(filePath)
	if err != nil {
		fail(fmt.Sprintf("erro ao obter informações do arquivo: %v", err))
		return
	}
	actualSize := info.Size()

	// 3. Valida o tamanho real contra o declarado
	if err := validateFileSize(actualSize, video.DeclaredSizeBytes); err != nil {
		fail(err.Error())
		return
	}

	// 4. Valida os magic bytes do contêiner
	valid, err := validateMagicBytes(filePath)
	if err != nil {
		fail(fmt.Sprintf("erro ao validar magic bytes: %v", err))
		return
	}
	if !valid {
		fail("arquivo não é um contêiner de vídeo reconhecido")
		return
	}

	// 5. Executa ffprobe com timeout de 5 segundos
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := runFFprobe(ctx, filePath); err != nil {
		fail(fmt.Sprintf("ffprobe falhou: %v", err))
		return
	}

	// Sucesso: marca como upload_complete e enfileira a transcodificação
	if err := models.SetUploadComplete(db, videoID, actualSize); err != nil {
		fail(fmt.Sprintf("erro ao marcar upload completo: %v", err))
		return
	}
	if err := enqueue(videoID); err != nil {
		fail(fmt.Sprintf("erro ao enfileirar transcodificação: %v", err))
		return
	}
	// Notifica que o vídeo entrou em processamento
	sendWebhook(videoID, "processing", "")
}
