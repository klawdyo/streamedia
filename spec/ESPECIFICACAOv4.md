# Media Server: Especificação Técnica Completa (V4)

Serviço de upload, transcodificação e entrega de vídeo em HLS, escrito em Go, para rodar em VPS via Coolify. Substitui as funcionalidades essenciais do Bunny Stream para uma rede social de vídeo no estilo Instagram com conteúdo vinculado a localização.

Este documento é a especificação para aprovação antes da implementação. Nenhum código será escrito antes da validação deste documento.

Mudanças da V2 em relação à V1: variáveis de ambiente migradas do docker-compose para um arquivo `.env` (com `.env.example` documentado e `.env` no gitignore), decisões antes em aberto agora confirmadas e fechadas.

Mudanças da V3 em relação à V2: correção da estratégia de variáveis de ambiente para o padrão correto do Coolify. O Coolify, após o build, gera automaticamente um arquivo `.env` no host a partir das variáveis configuradas no seu painel e o carrega no container. Para que o painel do Coolify descubra quais variáveis existem e crie os campos editáveis, a fonte de verdade precisa ser o bloco `environment` do `docker-compose.yml` usando a sintaxe `${VAR}`. Em desenvolvimento, o mesmo `.env` (criado manualmente a partir do `.env.example`) preenche esses `${VAR}` por interpolação. Assim o mesmo compose funciona em dev e em produção, e o painel do Coolify se popula sozinho a partir dele.

Mudanças da V4 em relação à V3: adição da convenção de idioma do código (seção 20). Código em inglês, comentários e mensagens ao usuário em português, com comentários abundantes mesmo nos trechos óbvios.

## Sumário

1. Visão geral e responsabilidades
2. Arquitetura e componentes
3. Modelo de dados (SQLite)
4. Fluxo completo de upload e reprodução
5. Autenticação e tokens
6. Ciclo de vida do vídeo (máquina de estados)
7. Fila de transcodificação
8. Jobs de manutenção
9. Rotas da API
10. Estrutura de arquivos no disco
11. Variáveis de ambiente e arquivo .env
12. Catálogo de falhas, bugs e mitigações
13. Validações
14. Plano de testes automatizados
15. Configuração Docker e Coolify
16. Conteúdo do README
17. GitHub Actions
18. Decisões fechadas
19. Payload do webhook
20. Convenção de idioma do código

## 1. Visão geral e responsabilidades

O media server tem uma responsabilidade única e bem delimitada: receber vídeos, transcodificá-los para múltiplas resoluções em formato HLS e servi-los quando requisitados com URL autenticada. Ele é deliberadamente "burro" quanto a usuários, permissões e relacionamento dos vídeos com posts. Toda essa lógica vive no backend principal.

Responsabilidades do media server:

- Gerar tokens de upload vinculados a um único `video_id`
- Receber upload em chunks via protocolo TUS
- Validar integridade e tipo do arquivo recebido
- Transcodificar para 480p, 720p e 1080p via FFmpeg
- Gerar a estrutura HLS no disco (master mais playlists e segmentos por resolução)
- Servir o `master.m3u8` apenas com token HMAC válido
- Servir playlists de resolução e segmentos como arquivos estáticos
- Notificar o backend principal via webhook nas transições de estado relevantes
- Processar tudo via fila para não derrubar a VPS
- Executar jobs de manutenção (matar uploads inativos, reenfileirar transcodes travados, limpar tokens)

Fora do escopo do media server:

- Saber quem é o usuário ou suas permissões
- Saber a qual post um vídeo pertence
- Decidir quem pode assistir um vídeo
- Qualquer interface administrativa rica (um dashboard mínimo é plus, não requisito)

## 2. Arquitetura e componentes

O serviço é um único binário Go, um único container. Internamente é dividido em:

- Servidor HTTP, usando a biblioteca padrão `net/http` com o roteador leve `chi`
- Handler TUS embutido (`tus/tusd` como biblioteca, não como serviço separado), gravando em disco local
- Validador de token HMAC em tempo constante
- Fila de transcodificação, um channel Go com buffer limitado mais um pool de workers
- Executor de FFmpeg via `os/exec`
- Camada de persistência SQLite, acessada via `database/sql` com o driver `modernc.org/sqlite` (driver em Go puro, sem CGo, simplifica o build Docker)
- Agendador de jobs de manutenção, goroutines com `time.Ticker`
- Cliente de webhook, com retry e log das tentativas

A escolha de Go sobre Node ou Rust se justifica por: tusd já é Go (embutível no mesmo processo), worker pool nativo via goroutines e channels, serving estático performático sem nginx na frente, HMAC na biblioteca padrão, e um único binário no container. O gargalo real é o FFmpeg, então a performance bruta da linguagem HTTP importa pouco.

## 3. Modelo de dados (SQLite)

SQLite foi escolhido em vez de arquivos `meta.json` em disco por três razões: atomicidade na reserva do `video_id` via constraint `UNIQUE`, consultas rápidas para o dashboard sem varrer o sistema de arquivos, e integridade verificável de estado independente da existência de pastas. A pasta existir não significa que o vídeo está pronto.

```sql
CREATE TABLE videos (
  video_id            TEXT PRIMARY KEY,
  status              TEXT NOT NULL DEFAULT 'pending_upload',
  declared_size_bytes INTEGER,
  actual_size_bytes   INTEGER,
  duration_s          INTEGER,
  resolutions         TEXT,
  transcode_attempts  INTEGER NOT NULL DEFAULT 0,
  last_chunk_at       DATETIME,
  error_message       TEXT,
  created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE upload_tokens (
  token      TEXT PRIMARY KEY,
  video_id   TEXT NOT NULL UNIQUE,
  expires_at DATETIME NOT NULL,
  FOREIGN KEY (video_id) REFERENCES videos(video_id)
);

CREATE TABLE webhook_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  video_id   TEXT NOT NULL,
  event      TEXT NOT NULL,
  payload    TEXT,
  sent_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  success    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_videos_status ON videos(status);
CREATE INDEX idx_videos_last_chunk ON videos(last_chunk_at);
CREATE INDEX idx_tokens_expires ON upload_tokens(expires_at);
```

Notas sobre os campos:

- `declared_size_bytes`: tamanho total que o cliente TUS declara no início do upload. Usado para validar contra `actual_size_bytes` ao final.
- `last_chunk_at`: timestamp do último chunk recebido. É o campo que o job de killer de uploads inativos usa para distinguir upload ativo de upload abandonado.
- `transcode_attempts`: controla o limite de retentativas automáticas de transcode.
- `resolutions`: JSON com as resoluções efetivamente geradas, por exemplo `[480,720,1080]`. Um vídeo de origem 720p não gera 1080p (não fazemos upscaling).
- O `UNIQUE` em `upload_tokens.video_id` garante que cada `video_id` tem no máximo um token ativo. Um UUID, um token, uma chance de upload.

SQLite deve ser aberto em modo WAL (`PRAGMA journal_mode=WAL`) para permitir leituras concorrentes durante escritas, e com `PRAGMA foreign_keys=ON`.

## 4. Fluxo completo de upload e reprodução

### 4.1 Upload

O `video_id` é gerado pelo Flutter, não pelo servidor. Isso é intencional: o Flutter associa o UUID ao arquivo local imediatamente após a captura, permitindo que o usuário reproduza o vídeo localmente enquanto o upload e o processamento acontecem em segundo plano. O usuário não gasta banda para enviar e depois baixar o que ele mesmo gravou.

```
1. Flutter captura o vídeo e gera um UUID v4 localmente
2. Flutter pede ao backend principal uma URL de upload, enviando o UUID
3. Backend principal valida que o UUID ainda não existe no seu banco
4. Backend principal chama o media server: POST /upload/init
   com o video_id e assinado com o secret compartilhado
5. Media server tenta inserir o video_id na tabela videos com status
   pending_upload. Se a primary key já existir, retorna 409
6. Media server gera um token de upload HMAC vinculado ao video_id,
   grava em upload_tokens e retorna a Upload-URL TUS ao backend principal
7. Backend principal repassa a Upload-URL ao Flutter
8. Flutter envia os chunks via TUS diretamente ao media server
9. A cada chunk, o media server atualiza last_chunk_at e status uploading
10. Ao concluir, o hook post-finish do tusd dispara:
    a. valida actual_size contra declared_size
    b. valida magic bytes do arquivo (é mesmo um vídeo?)
    c. status upload_complete, enfileira o transcode
    d. dispara webhook processing ao backend principal
11. Worker pega o job da fila, status transcoding, roda FFmpeg
12. FFmpeg gera as resoluções e a estrutura HLS no disco
13. status ready, dispara webhook ready ao backend principal
```

### 4.2 Reprodução

```
1. Flutter baixa a lista de posts do backend principal
2. Cada post traz uma URL no formato /api/play/{postId}
   (essa é uma rota do BACKEND PRINCIPAL, não do media server)
3. Quando o player vai abrir o vídeo, requisita /api/play/{postId}
4. Backend principal consulta o post, pega o video_id correspondente
5. Backend principal gera URL assinada para o master.m3u8 do media server
   com token HMAC e expiração
6. Backend principal responde com redirect 302 para a URL do media server
7. O player Flutter (video_player, better_player ou media_kit) abre o
   master.m3u8 normalmente, sem nenhuma customização
8. O player lê as resoluções e baixa os segmentos .ts diretamente
   (sem token, são estáticos públicos com nomes opacos)
```

## 5. Autenticação e tokens

Existem dois tipos de token, ambos baseados em HMAC-SHA256 com secret compartilhado entre backend principal e media server. Nenhum JWT, nenhuma biblioteca de autenticação externa, nenhuma sessão.

### 5.1 Token de upload

Vinculado a um único `video_id`. Gerado pelo media server na rota `/upload/init` e gravado na tabela `upload_tokens`. Esse token autoriza apenas o upload TUS daquele `video_id` específico. Expira em poucas horas (configurável). A constraint `UNIQUE` no `video_id` impede dois tokens ativos para o mesmo UUID.

### 5.2 Token de reprodução

Gerado pelo backend principal (que conhece o secret) para autorizar o acesso ao `master.m3u8`. Formato:

```
token = HMAC-SHA256(secret, video_id + ":" + expires)
URL = /videos/{video_id}/master.m3u8?expires={unix}&token={hex}
```

O media server valida assim:
- recalcula o HMAC com o secret e compara em tempo constante (`hmac.Equal`)
- verifica se `expires` ainda está no futuro

Decisão de design importante: somente o `master.m3u8` é autenticado. Os segmentos `.ts` e as playlists de resolução são servidos como arquivos estáticos públicos. A proteção do conteúdo está no acesso ao manifest master. Sem ele, o cliente não conhece os caminhos dos segmentos, que têm nomes opacos. Isso é o que serviços como o Bunny fazem na prática e evita ter que reescrever o `m3u8` em tempo de execução ou customizar o player.

### 5.3 Autorização entre backends (init)

A rota `/upload/init` também é protegida por HMAC. O backend principal assina a requisição com o mesmo secret. Opcionalmente, o token de reprodução pode carregar um campo `role` (`stream` ou `admin`) no payload do HMAC, permitindo que o media server libere rotas administrativas (listar vídeos, ver fila) apenas para tokens assinados com role admin. O media server não conhece usuários; ele apenas confia que o backend principal assinou corretamente.

## 6. Ciclo de vida do vídeo (máquina de estados)

```
pending_upload   → token gerado, aguardando o primeiro chunk
uploading        → chunks chegando, last_chunk_at sendo atualizado
failed_upload    → 10 min sem chunk novo; arquivos deletados; webhook enviado (TERMINAL)
upload_complete  → arquivo completo e validado, na fila de transcode
transcoding      → FFmpeg em execução
failed_transcode → FFmpeg falhou após N tentativas; webhook enviado (TERMINAL)
ready            → estrutura HLS pronta no disco; webhook enviado
```

Transições válidas:

```
pending_upload  → uploading | failed_upload
uploading       → uploading | upload_complete | failed_upload
upload_complete → transcoding
transcoding     → transcoding (reenfileiramento) | ready | failed_transcode
```

Estados terminais (`failed_upload`, `failed_transcode`) nunca são ressuscitados por job automático. A iniciativa de tentar de novo sempre parte do usuário, e gera um novo `video_id`, não reaproveita o antigo. O registro antigo permanece no banco com seu status terminal para fins de auditoria, mas sem arquivos no disco.

Justificativa do novo UUID em vez de retentativa no mesmo: reaproveitar o ID tornaria ambíguo distinguir "a falha original" de "a nova tentativa". Com novo UUID, o registro antigo fica auditável como `failed_upload`, o backend principal apenas atualiza a coluna do post para apontar ao novo UUID, e o media server trata como upload limpo sem herdar estado. O custo é uma atualização de coluna no backend principal.

## 7. Fila de transcodificação

A fila existe para impedir que múltiplos uploads simultâneos disparem múltiplos FFmpeg ao mesmo tempo e derrubem a VPS. O risco real não é memória (a fila guarda apenas referências a IDs, bytes), e sim contenção de CPU e disco, já que o FFmpeg é pesado.

Implementação:
- Um channel Go com buffer de tamanho `QUEUE_MAX_SIZE` (padrão 50)
- Um pool de `TRANSCODE_WORKERS` goroutines (padrão 1) consumindo da fila
- Ao chegar um upload completo, enfileira o `video_id`
- Se a fila estiver cheia, retorna erro adequado e marca o estado para reprocessamento posterior por job
- Os workers executam o FFmpeg sequencialmente até o limite de workers

O limite de fila é proteção contra abuso, não contra estouro de memória. Um valor de 50 jobs pendentes é folgado. Com 1 worker, transcodes acontecem um de cada vez, garantindo que a VPS não seja sobrecarregada.

Na inicialização do serviço, o sistema deve verificar entradas presas em `transcoding` ou `upload_complete` (resultado de um crash anterior) e reenfileirá-las ou tratá-las, para não perder jobs em caso de restart do container.

## 8. Jobs de manutenção

Três jobs, cada um com regra clara e testável. Rodam como goroutines com `time.Ticker`.

### Job 1: killer de uploads inativos (a cada 2 minutos)

```
regra:  status = uploading AND last_chunk_at < (agora - 10 minutos)
ação:   deleta arquivos parciais do disco
        status → failed_upload
        dispara webhook de falha
```

O prazo de 10 minutos é "sem chunk novo por 10 minutos", não "10 minutos desde o início". Um vídeo grande em conexão lenta continua enviando chunks e não é morto. O que é morto é o upload que travou de fato (fila travada, app fechado, conexão perdida sem retomada).

### Job 2: reenfileirador de transcodes travados (a cada 5 minutos)

```
regra:  status = transcoding AND updated_at < (agora - 30 minutos)
        AND transcode_attempts < 3
ação:   status → upload_complete (reenfileira)
        transcode_attempts + 1

regra:  status = transcoding AND updated_at < (agora - 30 minutos)
        AND transcode_attempts >= 3
ação:   status → failed_transcode
        dispara webhook de falha
```

Diferente do upload, o transcode pode ser reenfileirado automaticamente porque o arquivo de input ainda está no disco. O FFmpeg não tem checkpoint, então a retentativa roda do zero, mas não exige novo upload.

### Job 3: limpeza de tokens expirados (uma vez por dia)

```
regra:  expires_at < agora
ação:   deleta da tabela upload_tokens
        (o video_id permanece na tabela videos com seu status atual)
```

## 9. Rotas da API

### Rotas do media server

```
POST   /upload/init
       Protegida por HMAC entre backends.
       Body: { video_id, declared_size_bytes }
       Cria o registro, gera token de upload, retorna Upload-URL TUS.
       409 se o video_id já existir.

POST   /files/{video_id}        (endpoint TUS, criação)
PATCH  /files/{video_id}        (endpoint TUS, envio de chunk)
HEAD   /files/{video_id}        (endpoint TUS, consulta de offset para retomada)
       Protegidas pelo token de upload.

GET    /videos/{video_id}/master.m3u8?expires={}&token={}
       Autenticada por HMAC de reprodução. Serve o manifest master.

GET    /videos/{video_id}/{resolution}/playlist.m3u8
GET    /videos/{video_id}/{resolution}/{segment}.ts
       Estáticas públicas, nomes opacos.

GET    /api/status/{video_id}
       Retorna o status atual do vídeo (para polling ou debug).
       Protegida por HMAC.

GET    /admin/videos          (role admin)
       Lista vídeos e status. Faz parte do dashboard mínimo.

GET    /admin/queue           (role admin)
       Mostra a fila de transcode e os workers ativos.

GET    /healthz
       Health check para o Coolify.
```

### Rotas do backend principal (fora deste projeto, referência)

```
GET    /api/play/{postId}
       Consulta o post, pega o video_id, gera URL assinada,
       responde 302 para o media server.
```

## 10. Estrutura de arquivos no disco

```
/media/
  {video_id}/
    master.m3u8
    480/
      playlist.m3u8
      0.ts
      1.ts
      ...
    720/
      playlist.m3u8
      0.ts
      ...
    1080/
      playlist.m3u8
      0.ts
      ...
  .uploads/
    {video_id}              (arquivo bruto temporário durante upload TUS)
    {video_id}.info         (metadados TUS para retomada)
```

A pasta `.uploads` guarda os arquivos brutos durante o upload e o transcode. Após o `ready`, o arquivo bruto é deletado por padrão (`KEEP_ORIGINAL=false`). O `master.m3u8` aponta para as playlists de cada resolução com caminhos relativos.

Conteúdo do `master.m3u8`:

```
#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=900000,RESOLUTION=854x480
480/playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1280x720
720/playlist.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3500000,RESOLUTION=1920x1080
1080/playlist.m3u8
```

Bitrates alvo de cada resolução (valores padrão, configuráveis):

```
480p   → 854x480,   ~900 kbps de vídeo,  128 kbps de áudio
720p   → 1280x720,  ~2000 kbps de vídeo, 128 kbps de áudio
1080p  → 1920x1080, ~3500 kbps de vídeo, 192 kbps de áudio
```

O master.m3u8 só lista as resoluções efetivamente geradas. Não há upscaling: uma origem 720p gera só 480p e 720p, e o master lista apenas essas duas.

## 11. Variáveis de ambiente e arquivo .env

A estratégia de variáveis de ambiente segue o padrão do Coolify. Entender o mecanismo é importante para não duplicar configuração nem quebrar o deploy.

Como o Coolify trata variáveis: após o build, o Coolify gera um arquivo `.env` no host (em `/data/coolify/applications/{id}/.env`) contendo todas as variáveis de runtime configuradas no seu painel, e esse arquivo é carregado pelo Docker Compose na inicialização do container. Para que o painel do Coolify saiba quais variáveis existem e crie os campos editáveis, ele lê o `docker-compose.yml` e procura referências no formato `${VAR}`. Portanto, o `docker-compose.yml` é a fonte de verdade das variáveis, e o bloco `environment` declara cada uma usando `${VAR}` ou `${VAR:-padrao}`.

O fluxo nos dois ambientes:

- Em desenvolvimento: você copia `.env.example` para `.env` e preenche os valores. O `docker compose` carrega o `.env` automaticamente e usa esses valores para interpolar os `${VAR}` do bloco `environment`.
- Em produção (Coolify): você configura as variáveis no painel do Coolify. O Coolify gera o `.env` no host a partir do painel, e o compose interpola os mesmos `${VAR}`. Você não cria `.env` manualmente em produção; o painel é a fonte.

Regras de versionamento:
- `.env.example` é versionado no Git, com todas as variáveis comentadas e valores de exemplo (nunca secrets reais).
- `.env` fica no `.gitignore` e nunca é commitado.
- O `docker-compose.yml` declara cada variável no bloco `environment` com `${VAR}`, servindo de fonte de verdade para o painel do Coolify.

### Conteúdo de .env.example

```bash
# ==========================================================
# Media Server - variáveis de ambiente
# Em desenvolvimento: copie este arquivo para .env e ajuste os valores.
# Em produção (Coolify): configure pelo painel. O Coolify gera o .env
# automaticamente; não crie .env manualmente no servidor.
# As variáveis abaixo espelham os ${VAR} declarados no docker-compose.yml.
# ==========================================================

# Tamanho máximo de cada upload, em megabytes.
# Aplicado na criação do upload e verificado a cada chunk.
MAX_UPLOAD_SIZE_MB=10

# Secret HMAC compartilhado entre o backend principal e o media server.
# OBRIGATÓRIO. Gere com: openssl rand -hex 32
# O serviço falha ao iniciar se esta variável estiver ausente.
UPLOAD_TOKEN_SECRET=troque_por_um_secret_de_64_chars_hex

# Diretório onde ficam os arquivos HLS finais (volume persistente).
MEDIA_DIR=/media

# Diretório dos uploads em andamento (arquivos brutos TUS).
UPLOAD_TMP_DIR=/media/.uploads

# Caminho do banco SQLite (volume persistente, separado da mídia).
SQLITE_PATH=/data/media.db

# Tamanho máximo da fila de transcode. Protege contra abuso.
QUEUE_MAX_SIZE=50

# Número de workers FFmpeg simultâneos.
# Mantenha em 1 em VPS modesta para não sobrecarregar a CPU.
TRANSCODE_WORKERS=1

# Validade do token de upload, em horas.
UPLOAD_TOKEN_TTL_H=6

# Validade máxima aceita no token de reprodução, em horas.
# Tokens com expiração além disso são rejeitados.
PLAY_TOKEN_MAX_TTL_H=6

# Minutos sem receber um novo chunk antes de matar o upload.
UPLOAD_IDLE_TIMEOUT_MIN=10

# Minutos para considerar um transcode travado e reenfileirar.
TRANSCODE_STUCK_MIN=30

# Número de tentativas de transcode antes de marcar failed_transcode.
MAX_TRANSCODE_ATTEMPTS=3

# URL do backend principal que recebe os webhooks. OBRIGATÓRIO.
WEBHOOK_URL=https://seu-backend.exemplo.com/webhooks/media

# Secret para assinar os webhooks enviados ao backend principal.
# OBRIGATÓRIO. Gere com: openssl rand -hex 32
WEBHOOK_SECRET=troque_por_outro_secret_de_64_chars_hex

# Manter o arquivo bruto original após o transcode.
# false economiza disco; true permite reprocessar sem novo upload.
KEEP_ORIGINAL=false

# Porta HTTP do serviço.
PORT=3000

# Limite de requisições por IP por minuto.
RATE_LIMIT_PER_MIN=60
```

### Conteúdo de .gitignore

```
# Ambiente
.env

# Binários
/mediaserver
*.exe

# Banco e dados locais de dev
*.db
*.db-wal
*.db-shm
/data/
/media/

# Go
/vendor/

# Editor
.idea/
.vscode/
.DS_Store
```

Variáveis obrigatórias, que fazem o serviço falhar na inicialização se ausentes: `UPLOAD_TOKEN_SECRET`, `WEBHOOK_URL`, `WEBHOOK_SECRET`. As demais têm padrão sensato, expresso no compose via `${VAR:-padrao}`.

Nota sobre o caractere `$` em secrets no Coolify: ao colar secrets gerados no painel do Coolify, marque a opção "Is Literal" para evitar que caracteres especiais como `$` sejam interpretados como interpolação. Secrets hex gerados com `openssl rand -hex 32` não contêm `$`, então o risco é baixo, mas a recomendação vale como boa prática.

## 12. Catálogo de falhas, bugs e mitigações

### 12.1 Segurança

| Falha | Risco | Mitigação |
|---|---|---|
| Path traversal no video_id | Sobrescrita de arquivos arbitrários do sistema | Validar que o video_id é um UUID v4 estrito via regex antes de qualquer uso em path. Rejeitar qualquer caractere fora de hex e hífen. |
| Upload sem limite de tamanho | Disco cheio, serviço derrubado | Limite via MAX_UPLOAD_SIZE_MB aplicado na criação do upload TUS e verificado a cada chunk. |
| Arquivo que não é vídeo | Desperdício de fila e CPU, possível payload malicioso | Verificar magic bytes do container (ftyp para MP4/MOV, etc.) após o upload completo, antes de enfileirar. Não confiar no Content-Type do cliente. |
| Timing attack na validação do HMAC | Vazamento do secret por análise de tempo | Comparação em tempo constante com hmac.Equal, nunca com == de strings. |
| Token sem expiração curta | Conteúdo exposto indefinidamente se o token vazar | Expiração máxima de 6 horas no master.m3u8, validada contra PLAY_TOKEN_MAX_TTL_H. |
| SSRF via FFmpeg | Requisições internas na rede via input de URL | Nunca passar input controlado pelo usuário como argumento do FFmpeg. Somente o path local do arquivo já salvo e validado. |
| Directory listing | Exposição de todos os video_ids | Desabilitar listagem de diretórios explicitamente no handler de estáticos. |
| Secret fraco ou commitado | Comprometimento total da autenticação | Secret via variável de ambiente, nunca hardcoded, .env no gitignore. Gerar com openssl rand -hex 32 (256 bits). Serviço falha ao iniciar se ausente. |
| Container rodando como root | Escalada de privilégio em caso de exploit | Usuário sem privilégios no Dockerfile, processo roda com ele. |
| Sem rate limiting | Flood de requisições de upload | Rate limiting por IP no próprio handler (RATE_LIMIT_PER_MIN). |
| Webhook spoofado | Backend principal recebe notificação falsa | Assinar os webhooks enviados com WEBHOOK_SECRET via HMAC, backend principal valida. |

### 12.2 Integridade e estado

| Falha | Risco | Mitigação |
|---|---|---|
| Race condition na criação do video_id | Duas requisições com o mesmo UUID criam estado duplicado | Constraint UNIQUE na primary key; a inserção é a operação de reserva. Conflito retorna 409. |
| Pasta existe mas vídeo não está pronto | Servir vídeo corrompido ou incompleto | Reprodução verifica status = ready no banco, nunca a existência da pasta ou do m3u8. |
| Tamanho declarado diverge do recebido | Arquivo truncado processado como válido | Comparar actual_size_bytes com declared_size_bytes no hook post-finish. Divergência rejeita. |
| Crash do serviço durante transcode | Job perdido, entrada presa em transcoding | Na inicialização, varrer entradas em transcoding e upload_complete e reenfileirar. |
| Crash durante upload | Entrada presa em uploading | Job killer mata após 10 min sem chunk; na inicialização também pode ser verificado. |
| Transcode reenfileirado infinitamente | Loop de falha consumindo CPU | Limite MAX_TRANSCODE_ATTEMPTS; ao exceder, vira failed_transcode terminal. |
| Reaproveitar UUID de falha | Ambiguidade entre falha original e retentativa | Retentativa sempre gera novo UUID; registro antigo permanece terminal e auditável. |
| Escrita concorrente no SQLite | Database is locked | Modo WAL, e serializar escritas críticas. Driver em Go puro (modernc) lida bem com WAL. |
| Webhook falha de entrega | Backend principal não sabe do ready | Retry com backoff, registro em webhook_log com flag success. Rota de status permite reconciliação. |
| FFmpeg trava sem retornar | Worker bloqueado indefinidamente | Timeout no os/exec com context; ao estourar, mata o processo e conta como tentativa. |

### 12.3 Operacional

| Falha | Risco | Mitigação |
|---|---|---|
| Disco enche com originais | Serviço para de aceitar uploads | KEEP_ORIGINAL=false deleta o bruto após transcode. Monitorar espaço. |
| Lixo de uploads abandonados | Acúmulo de arquivos parciais | Job killer deleta arquivos ao marcar failed_upload. |
| Volume não persistido no Coolify | Perda de vídeos ao atualizar o backend | Volumes nomeados separados para media e db, mapeados a paths persistentes no Coolify. |
| Atualização do backend apaga estáticos | Vídeos somem em deploy | Volumes desacoplados do código do container; o deploy só troca a imagem. |
| Migração de schema quebra | Banco incompatível após update | Migrações versionadas executadas na inicialização, idempotentes. |

## 13. Validações

Validações de entrada, aplicadas em ordem, com rejeição imediata na primeira que falhar:

Na rota `/upload/init`:
- assinatura HMAC entre backends válida
- `video_id` é UUID v4 estrito (regex)
- `video_id` ainda não existe no banco (senão 409)
- `declared_size_bytes` maior que zero e menor ou igual ao limite configurado

Durante o upload TUS:
- token de upload válido e não expirado
- token corresponde ao `video_id` da URL
- tamanho acumulado dos chunks não excede o limite nem o `declared_size_bytes`

No hook post-finish:
- `actual_size_bytes` igual ao `declared_size_bytes` (tolerância zero ou mínima)
- magic bytes correspondem a um container de vídeo suportado
- o FFprobe consegue ler o arquivo e extrair duração e dimensões válidas

Na reprodução:
- HMAC de reprodução válido em tempo constante
- `expires` no futuro e dentro do TTL máximo
- `video_id` é UUID v4 estrito
- vídeo está em status `ready`

Na resolução do transcode:
- não fazer upscaling: gerar apenas resoluções menores ou iguais à origem
- um vídeo 720p gera 480p e 720p, nunca 1080p

## 14. Plano de testes automatizados

Testes unitários e de integração com a biblioteca padrão `testing` do Go, usando um SQLite em memória ou temporário por teste.

### Autenticação
- HMAC de reprodução válido aceita; HMAC adulterado rejeita
- token expirado rejeita; token no futuro além do TTL máximo rejeita
- comparação em tempo constante (verificar uso de hmac.Equal, não ==)
- HMAC entre backends válido aceita init; assinatura errada rejeita

### Criação e reserva de ID
- video_id novo cria registro pending_upload e token
- video_id duplicado retorna 409 e não cria segundo registro
- video_id não-UUID rejeita
- declared_size acima do limite rejeita

### Validação de arquivo
- arquivo com magic bytes de MP4 passa
- arquivo de texto disfarçado de vídeo rejeita
- arquivo truncado (actual != declared) rejeita

### Máquina de estados
- transições válidas são aceitas
- transições inválidas são bloqueadas (ex: ready não volta para uploading)
- estados terminais não são alterados por jobs

### Job killer de uploads inativos
- upload com last_chunk_at de 11 min atrás vira failed_upload e deleta arquivos
- upload com last_chunk_at de 9 min atrás não é tocado
- upload com last_chunk_at de 11 min mas status ready não é tocado
- ao matar, dispara webhook de falha

### Job reenfileirador de transcode
- transcoding há 31 min com 0 tentativas reenfileira e incrementa attempts
- transcoding há 31 min com 3 tentativas vira failed_transcode e dispara webhook
- transcoding há 29 min não é tocado

### Job limpeza de tokens
- token expirado é deletado
- token válido é mantido
- deletar token não altera o status do vídeo

### Fila
- enfileirar até o limite funciona
- enfileirar além do limite retorna erro adequado e não trava
- com 1 worker, dois jobs rodam em sequência, não em paralelo

### Recuperação de crash
- na inicialização, entradas em transcoding são reenfileiradas
- na inicialização, entradas em upload_complete são reenfileiradas

### Path traversal
- video_id com ../ é rejeitado antes de qualquer acesso a disco
- resolution fora de {480,720,1080} é rejeitada

### Não-upscaling
- origem 480p gera só 480p
- origem 720p gera 480p e 720p
- origem 1080p ou maior gera as três

### Webhook
- ready dispara webhook com payload correto e assinatura
- falha de entrega registra em webhook_log com success=0 e tenta retry
- a assinatura do webhook é verificável com o WEBHOOK_SECRET

## 15. Configuração Docker e Coolify

### docker-compose.yml

O bloco `environment` é a fonte de verdade das variáveis, declaradas com `${VAR}` para que o painel do Coolify as descubra. Em dev, o `docker compose` lê o `.env` automaticamente e interpola esses valores. Em produção, o Coolify gera o `.env` a partir do painel e interpola da mesma forma.

```yaml
services:
  mediaserver:
    build: .
    restart: unless-stopped
    environment:
      # Obrigatórias (sem padrão; o serviço falha ao iniciar se ausentes)
      - UPLOAD_TOKEN_SECRET=${UPLOAD_TOKEN_SECRET}
      - WEBHOOK_URL=${WEBHOOK_URL}
      - WEBHOOK_SECRET=${WEBHOOK_SECRET}
      # Com padrão sensato via ${VAR:-padrao}
      - MAX_UPLOAD_SIZE_MB=${MAX_UPLOAD_SIZE_MB:-10}
      - MEDIA_DIR=${MEDIA_DIR:-/media}
      - UPLOAD_TMP_DIR=${UPLOAD_TMP_DIR:-/media/.uploads}
      - SQLITE_PATH=${SQLITE_PATH:-/data/media.db}
      - QUEUE_MAX_SIZE=${QUEUE_MAX_SIZE:-50}
      - TRANSCODE_WORKERS=${TRANSCODE_WORKERS:-1}
      - UPLOAD_TOKEN_TTL_H=${UPLOAD_TOKEN_TTL_H:-6}
      - PLAY_TOKEN_MAX_TTL_H=${PLAY_TOKEN_MAX_TTL_H:-6}
      - UPLOAD_IDLE_TIMEOUT_MIN=${UPLOAD_IDLE_TIMEOUT_MIN:-10}
      - TRANSCODE_STUCK_MIN=${TRANSCODE_STUCK_MIN:-30}
      - MAX_TRANSCODE_ATTEMPTS=${MAX_TRANSCODE_ATTEMPTS:-3}
      - KEEP_ORIGINAL=${KEEP_ORIGINAL:-false}
      - PORT=${PORT:-3000}
      - RATE_LIMIT_PER_MIN=${RATE_LIMIT_PER_MIN:-60}
    volumes:
      - media_files:/media
      - db_data:/data
    ports:
      - "${PORT:-3000}:3000"
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:3000/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  media_files:
  db_data:
```

Não se usa `env_file` explícito aqui: em dev o `docker compose` já carrega o `.env` do diretório automaticamente para a interpolação dos `${VAR}`, e em produção o Coolify cuida disso. Declarar `env_file` apontando para um `.env` que não existe em dev (quando o usuário ainda não copiou do example) abortaria o run, então a interpolação via `environment` é mais segura.

### Dockerfile (multi-stage)

```dockerfile
# build
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mediaserver ./cmd/server

# runtime
FROM alpine:3.20
RUN apk add --no-cache ffmpeg wget && \
    adduser -D -u 10001 appuser
COPY --from=build /mediaserver /usr/local/bin/mediaserver
USER appuser
EXPOSE 3000
ENTRYPOINT ["mediaserver"]
```

O driver SQLite em Go puro (modernc.org/sqlite) permite CGO_ENABLED=0, gerando um binário estático que roda em Alpine sem dependências de C. O FFmpeg é instalado via apk.

### Notas Coolify

- Os dois volumes (`media_files` e `db_data`) devem ser mapeados a paths persistentes nas configurações de storage do serviço no Coolify, para sobreviver a redeploys.
- As variáveis de ambiente são configuradas no painel de environment do Coolify. O Coolify lê os `${VAR}` declarados no bloco `environment` do docker-compose e cria os campos editáveis no painel automaticamente. No deploy, ele gera o `.env` no host e o compose interpola os valores. Você não cria `.env` manualmente em produção.
- Os secrets (`UPLOAD_TOKEN_SECRET`, `WEBHOOK_SECRET`) devem ser marcados como secret no painel do Coolify, e com a opção "Is Literal" ativada para evitar interpretação de caracteres especiais.
- O health check em `/healthz` permite ao Coolify saber se o serviço está saudável.
- Deploys trocam apenas a imagem do container; os volumes permanecem intactos, então atualizar o backend não mexe nos arquivos estáticos nem no banco.

## 16. Conteúdo do README

O README a ser gerado na implementação deve conter:

- Descrição do projeto e seu papel na arquitetura geral (media server desacoplado do backend principal)
- Diagrama do fluxo de upload e reprodução
- Pré-requisitos (Docker, FFmpeg embutido na imagem)
- Como copiar `.env.example` para `.env` em dev e como gerar os secrets (openssl rand -hex 32); e como, em produção, o Coolify gera o `.env` a partir do painel
- Tabela completa de variáveis de ambiente com descrição e padrão (espelhando o .env.example)
- Como rodar localmente com docker compose
- Como buildar a imagem manualmente
- Explicação de cada volume: media_files (arquivos HLS e uploads), db_data (SQLite)
- Explicação da estrutura de pastas no disco
- Documentação de cada rota da API com exemplos de request e response
- Como gerar um token de reprodução do lado do backend principal (exemplo de código do HMAC)
- Formato do payload dos webhooks enviados e como validar a assinatura (referência à seção 19)
- Como rodar os testes (go test ./...)
- Seção de deploy no Coolify passo a passo, explicando que o painel é a fonte das variáveis e que o Coolify gera o `.env` automaticamente a partir dos `${VAR}` do compose
- Seção de troubleshooting (transcode travado, disco cheio, banco locked)
- Tabela de estados do vídeo e seus significados
- Nota sobre a convenção de idioma do projeto (código em inglês, comentários e mensagens ao usuário em português), referenciando a seção 20

## 17. GitHub Actions

Dois workflows:

### ci.yml (em todo push e pull request)

```
- checkout
- setup-go
- go vet ./...
- go test ./... com cobertura
- build do binário para garantir que compila
- golangci-lint
```

### release.yml (em tag de versão)

```
- checkout
- setup do Docker Buildx
- login no GitHub Container Registry (ghcr.io) usando o GITHUB_TOKEN nativo
- build multi-stage da imagem
- push com a tag da versão e latest
```

A imagem publicada no ghcr.io pode então ser referenciada diretamente no Coolify, ou o Coolify pode buildar do repositório. O README documentará as duas opções.

Nenhum secret de ambiente da aplicação entra nos workflows. Os secrets do GitHub usados são apenas os de publicação (o GITHUB_TOKEN nativo já cobre o push para ghcr.io).

## 18. Decisões fechadas

Todos os pontos antes em aberto agora estão confirmados:

1. Roteador HTTP: `chi`.
2. Geração do master.m3u8: manual, é texto simples, controle total.
3. Arquivo original após transcode: deletado por padrão (`KEEP_ORIGINAL=false`).
4. Bitrates por resolução: valores padrão definidos na seção 10 (480p ~900k, 720p ~2000k, 1080p ~3500k), configuráveis.
5. Dashboard mínimo: entra na primeira versão como rotas `/admin/videos` e `/admin/queue`, protegidas por token de role admin.
6. Payload do webhook: especificado na seção 19.

## 19. Payload do webhook

O media server envia um POST ao `WEBHOOK_URL` em cada transição relevante de estado. O corpo é JSON e vai assinado.

Eventos enviados:
- `processing`: upload concluído e validado, transcode enfileirado
- `ready`: estrutura HLS pronta, vídeo pode ser servido
- `failed`: falha terminal (upload ou transcode), com o motivo

Corpo do POST:

```json
{
  "video_id": "uuid-v4-do-video",
  "event": "ready",
  "status": "ready",
  "duration_s": 47,
  "resolutions": [480, 720, 1080],
  "error_message": null,
  "timestamp": "2026-06-05T10:00:00Z"
}
```

Para o evento `failed`, `status` será `failed_upload` ou `failed_transcode`, e `error_message` traz o motivo. Os campos `duration_s` e `resolutions` podem ser nulos quando não aplicáveis.

Assinatura do webhook:

```
header: X-Signature: sha256={hex}
valor:  HMAC-SHA256(WEBHOOK_SECRET, corpo_json_bruto)
```

O backend principal valida recalculando o HMAC sobre o corpo bruto recebido e comparando em tempo constante. Requisições com assinatura inválida devem ser descartadas pelo backend principal.

Entrega e retry: cada tentativa é registrada na tabela `webhook_log` com a flag `success`. Em caso de falha de entrega (timeout ou status não-2xx), o media server tenta novamente com backoff. Se todas as tentativas falharem, o registro fica com `success=0` e a rota `/api/status/{video_id}` permite ao backend principal reconciliar o estado por polling.

## 20. Convenção de idioma do código

O código segue uma separação estrita entre idioma de identificadores e idioma de comunicação.

Em inglês, sempre:
- nomes de variáveis, funções, métodos, tipos, structs, interfaces e constantes
- nomes de arquivos e pacotes
- nomes de colunas e tabelas do banco
- chaves de JSON em payloads de API e webhook
- nomes de rotas e parâmetros de query
- nomes de variáveis de ambiente
- logs internos de debug destinados ao desenvolvedor

Em português, sempre:
- todos os comentários no código, sem exceção
- mensagens de erro retornadas em respostas de API que possam ser exibidas ao usuário final ou consumidas pelo backend principal para exibição
- o README e qualquer documentação voltada a pessoas
- os comentários do `.env.example`
- textos de ajuda de linha de comando, se houver

Densidade de comentários: o código deve ser fortemente comentado, mesmo em trechos óbvios. A intenção é que alguém lendo o código entenda cada passo sem precisar inferir. Comentar o porquê de cada decisão, não apenas o que a linha faz. Em blocos de lógica de negócio (máquina de estados, validações, jobs de manutenção, geração e validação de HMAC), cada etapa relevante recebe um comentário explicativo. Preferir comentários acima do bloco ou da linha, em português claro e direto.

Exemplo do estilo esperado:

```go
// Valida que o video_id é um UUID v4 estrito antes de qualquer uso em path.
// Isso previne path traversal: um id com "../" poderia escapar do diretório
// de mídia e sobrescrever arquivos arbitrários do sistema.
func isValidVideoID(id string) bool {
	// Regex casa exatamente o formato UUID v4: 8-4-4-4-12 dígitos hex,
	// com o 13o dígito fixo em 4 e o 17o entre 8, 9, a ou b.
	return uuidV4Pattern.MatchString(id)
}
```

Observe no exemplo: o identificador da função (`isValidVideoID`) e o tipo (`string`, `bool`) estão em inglês, enquanto toda a explicação está em português, cobrindo inclusive o detalhe óbvio do que a regex casa. Esse é o padrão a seguir em todo o projeto.

Mensagens de erro: erros internos de log podem ficar em inglês por convenção de ecossistema Go, mas qualquer mensagem que chegue ao consumidor da API (corpo de resposta de erro, campo `error_message` do webhook) deve estar em português, já que o backend principal pode repassá-la à interface. Exemplo de resposta de erro da API:

```json
{
  "error": "O vídeo já existe e não pode ser enviado novamente."
}
```

Após a aprovação deste documento, a implementação seguirá exatamente estas especificações, com a estrutura de pastas, o código Go, os testes, o Dockerfile, o docker-compose, o `.env.example`, o `.gitignore`, o README e os workflows do GitHub Actions.
