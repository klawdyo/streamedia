# T24: README.md

**Status:** pending
**Dependências:** T22
**Estimativa:** média

## Contexto

O README é a documentação voltada a pessoas. Deve ser escrito em português,
conforme a convenção de idioma do projeto.

## Conteúdo obrigatório

### 1. Descrição e papel na arquitetura

- O que é o Streamedia media server
- Como ele se encaixa: backend principal → media server → Flutter
- O que é responsabilidade do media server (e o que não é)

### 2. Diagrama de fluxo

Use diagrama em texto ASCII ou mermaid:

```
Flutter → Backend Principal → POST /upload/init → Media Server
Flutter → TUS chunks diretamente → Media Server
Media Server → FFmpeg → HLS
Media Server → Webhook → Backend Principal
```

### 3. Pré-requisitos

- Docker e docker-compose
- (FFmpeg embutido na imagem, não precisa instalar)

### 4. Desenvolvimento local

```bash
# Clone e configure
cp .env.example .env
# Edite .env com seus valores
# Gere secrets:
openssl rand -hex 32  # para UPLOAD_TOKEN_SECRET
openssl rand -hex 32  # para WEBHOOK_SECRET
openssl rand -hex 32  # para ADMIN_TOKEN

# Suba o container
docker compose up
```

### 5. Tabela completa de variáveis de ambiente

Espelha o .env.example com colunas: Variável | Obrigatória | Padrão | Descrição

### 6. Deploy no Coolify (passo a passo)

- O Coolify lê os `${VAR}` do docker-compose.yml e cria campos no painel
- Configure as variáveis no painel (secrets marcados como "Is Literal")
- Configure os volumes: media_files e db_data com paths persistentes
- O Coolify gera .env automaticamente — não crie .env manualmente em produção

### 7. Documentação das rotas da API

Com exemplos de request e response para cada rota:
- `POST /upload/init`
- TUS routes (`/files/{video_id}`)
- `GET /videos/{video_id}/master.m3u8`
- `GET /api/status/{video_id}`
- `GET /admin/videos`
- `GET /admin/queue`
- `GET /healthz`

### 8. Como gerar token de reprodução (exemplo do backend principal)

```go
// Exemplo em Go — lógica que roda no BACKEND PRINCIPAL
func generatePlayURL(secret, videoID string, ttl time.Duration) string {
    expires := time.Now().Add(ttl).Unix()
    payload := fmt.Sprintf("%s:%d", videoID, expires)
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(payload))
    token := hex.EncodeToString(mac.Sum(nil))
    return fmt.Sprintf("/videos/%s/master.m3u8?expires=%d&token=%s", videoID, expires, token)
}
```

### 9. Formato do webhook

Payload JSON completo com exemplos para cada event (processing, ready, failed).
Como validar a assinatura `X-Signature`.

### 10. Como rodar os testes

```bash
go test ./...
go test ./... -v  # verbose
go test ./... -run TestNome  # teste específico
```

### 11. Tabela de estados do vídeo

| Status | Descrição |
|--------|-----------|
| pending_upload | Token gerado, aguardando primeiro chunk |
| uploading | Chunks chegando |
| upload_complete | Arquivo completo e validado |
| transcoding | FFmpeg em execução |
| ready | HLS pronto para servir |
| failed_upload | Falha terminal no upload |
| failed_transcode | Falha terminal na transcodificação |

### 12. Troubleshooting

- Transcode travado: verificar webhook_log, rodar `GET /admin/queue`
- Disco cheio: `KEEP_ORIGINAL=false` e verificar `.uploads`
- Banco locked: WAL mode está ativo? Verificar múltiplos processos
- Token inválido: verificar sincronização de relógios entre backend e media server

### 13. Convenção de idioma

Nota sobre código em inglês, comentários em português.

## QA Instructions

Crie `readme_test.go` na raiz:

```
TestReadmeExists
  - Verifica que README.md existe

TestReadmeSections
  - Lê README.md
  - Verifica presença das seções obrigatórias:
    - "Coolify" (seção de deploy)
    - "POST /upload/init"
    - "master.m3u8"
    - "webhook"
    - "go test"
    - "UPLOAD_TOKEN_SECRET"
    - status dos vídeos (tabela)

TestReadmeNotEmpty
  - README.md tem mais de 1000 bytes (não é placeholder)
```

## Dev Instructions

Escreva o README.md completo em português, com:
- Markdown bem formatado
- Código em blocos de código com syntax highlighting
- Diagrama de fluxo (ASCII art ou mermaid)
- Linguagem clara e direta — é para desenvolvedores

## Arquivos a criar

- `README.md`
- `readme_test.go`

## Definition of Done

- [ ] Todas as seções obrigatórias presentes
- [ ] Exemplos de código funcionais
- [ ] Deploy no Coolify explicado claramente
- [ ] Todos os testes passam
