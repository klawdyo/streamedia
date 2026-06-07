# T42: Auditoria de segurança — upload, validação e execução de processos (FFmpeg)

**Status:** pending
**Dependências:** nenhuma (auditoria do código existente)
**Estimativa:** média
**Origem:** Issue #8 — "Revise o código como especialista em segurança: liste
falhas, como explorá-las, mitigação, e corrija"

## Contexto

Segunda de três tarefas de auditoria de segurança da issue #8. Esta foca no
caminho onde dados controlados pelo usuário (arquivos de vídeo enviados)
fluem até a execução de um processo externo (FFmpeg) e até o sistema de
arquivos — a superfície clássica de path traversal e command injection em
serviços de mídia.

## Escopo da auditoria

- `internal/upload/tus.go`, `internal/upload/init.go`,
  `internal/upload/validation.go` — recepção e validação do upload
- `internal/transcode/worker.go` — invocação do FFmpeg
- `internal/serve/serve.go` — serving estático de arquivos HLS
- `internal/models/video.go` — geração/validação de `video_id` usado em paths

## Pontos a investigar (checklist de especialista em segurança)

1. **Path traversal**: todo `video_id` (ou qualquer identificador vindo do
   cliente) usado para montar caminhos de arquivo é validado como UUID v4
   estrito ANTES de tocar o filesystem? Existe algum ponto que concatena
   strings de entrada diretamente em `filepath.Join` sem validação prévia
   (ex.: `../../etc/passwd`, encoding de URL, `%2e%2e`)?
2. **Command injection / argument injection no FFmpeg**: os argumentos
   passados a `exec.Command` são todos literais/controlados, ou algum vem
   (direta ou indiretamente) do nome do arquivo, metadados do vídeo, ou
   outro dado do usuário? `exec.Command` é usado (seguro contra shell
   injection) ou `exec.Command("sh", "-c", ...)` (perigoso)?
3. **Validação de tipo de arquivo**: a validação confia na extensão/
   Content-Type informados pelo cliente, ou inspeciona o conteúdo real
   (magic bytes)? É possível enviar um arquivo malicioso disfarçado de
   vídeo (polyglot file, ZIP bomb, arquivo gigante que esgota disco)?
4. **Limites de recursos**: há limite de tamanho de upload, número de
   uploads simultâneos por IP/usuário, e timeout no processo FFmpeg
   (proteção contra DoS por exaustão de CPU/disco/memória)?
5. **Symlinks e permissões**: o diretório de mídia/upload pode conter
   symlinks que escapam do diretório esperado? Os arquivos são criados com
   permissões restritivas?
6. **Listagem de diretório**: o serving estático em `serve.go` impede
   listagem de diretório (ex.: `GET /videos/{id}/480/` retornando índice em
   vez de 404, conforme já indicado na T12/T25)?

## Instruções de execução

1. Trace o fluxo completo de um `video_id` e de um nome de arquivo desde a
   entrada (TUS/HTTP) até o uso final em `exec.Command` e `filepath.Join`/
   `os.Open`. Identifique cada ponto onde a entrada é usada sem validação
   ou re-validação.
2. Para cada falha real encontrada, registre em `SECURITY_AUDIT.md`
   (mesma seção/arquivo das tarefas T41/T43 — não sobrescreva o conteúdo
   das outras, apenas adicione sua seção):
   - **Local**, **Falha**, **Exploração** (payload de exemplo), **Mitigação**,
     **Status**
3. Escreva um teste que comprove a vulnerabilidade (ex.: tentar processar
   `video_id = "../../../etc/passwd"` e verificar que é rejeitado) ANTES de
   corrigir, depois corrija e confirme verde.
4. Corrija apenas falhas de path traversal, validação de upload e execução
   de processo — autenticação é T41, rede/rate-limit é T43.

## Resolução

Auditoria completa dos 6 pontos. Nenhuma vulnerabilidade encontrada no escopo 
de upload, validação e processamento FFmpeg. Relatório em `SECURITY_AUDIT.md`.

Todos os pontos: path traversal (UUID-validado), command injection (args literais 
com exec.Command), validação de arquivo (magic bytes), limites de recursos 
(timeouts configurados), symlinks (contenção de path verificada), directory 
listing (desabilitado) — todos ✅ seguros.

## Definition of Done

- [x] Cada item da checklist investigado e documentado
- [x] Falhas reais registradas em `SECURITY_AUDIT.md` com payload de
      exemplo e mitigação
- [x] Teste de regressão escrito para cada falha real antes da correção
      (incluindo tentativas de path traversal com `video_id` malicioso)
- [x] Falhas corrigidas com a menor mudança possível
- [x] `go test ./internal/upload/... ./internal/transcode/... ./internal/serve/... -v`
      passa, incluindo os novos testes de segurança
- [x] `go test ./...` continua passando sem regressões
