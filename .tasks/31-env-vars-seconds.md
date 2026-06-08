# T31: Padronizar variáveis de tempo das envs em segundos

**Status:** done
**Dependências:** nenhuma
**Estimativa:** pequena
**Issue relacionada:** #4

## Contexto

A issue #4 aponta uma inconsistência: hoje as variáveis de ambiente de tempo
misturam unidades — `UPLOAD_TOKEN_TTL_H` e `PLAY_TOKEN_MAX_TTL_H` em horas,
`UPLOAD_IDLE_TIMEOUT_MIN` e `TRANSCODE_STUCK_MIN` em minutos
(`internal/config/config.go:64-76`). Isso obriga quem configura o serviço a
lembrar a unidade de cada variável individualmente. A convenção pedida é
segundos para todas.

Tarefa **independente e de baixo risco** — pode ser feita a qualquer momento,
mas vale a pena adiantar antes das tarefas maiores (T32+) para não ter que
mexer em `config.go` duas vezes.

## Dev Instructions

- Renomeie as variáveis para o sufixo `_SECONDS`:
  - `UPLOAD_TOKEN_TTL_H` → `UPLOAD_TOKEN_TTL_SECONDS`
  - `PLAY_TOKEN_MAX_TTL_H` → `PLAY_TOKEN_MAX_TTL_SECONDS`
  - `UPLOAD_IDLE_TIMEOUT_MIN` → `UPLOAD_IDLE_TIMEOUT_SECONDS`
  - `TRANSCODE_STUCK_MIN` → `TRANSCODE_STUCK_SECONDS`
- Ajuste os valores-padrão para o equivalente em segundos (ex.: 6h → 21600,
  10min → 600, 30min → 1800) e troque `time.Hour`/`time.Minute` por
  `time.Second` na construção dos `time.Duration` em `config.go`.
- **Não** mantenha aliases das variáveis antigas — é uma mudança incompatível
  intencional (breaking change de configuração); documente isso no
  `README.md` e no changelog/notas de release, para quem for atualizar uma
  instalação existente.
- Atualize `.env.example`, `docker-compose.yml`/`Dockerfile` (se citarem
  essas variáveis) e a tabela de variáveis de ambiente no `README.md`.
- Atualize `internal/config/config_test.go` com os novos nomes e valores.

## QA Instructions

- Atualize/crie testes em `internal/config/config_test.go`:
  - `TestLoadConfig_TimeVarsInSeconds`: define `UPLOAD_TOKEN_TTL_SECONDS=900`
    etc. via `os.Setenv` e verifica que `cfg.UploadTokenTTL == 900*time.Second`
    (idem para os outros três).
  - `TestLoadConfig_DefaultsAreInSeconds`: sem as envs definidas, verifica
    que os defaults batem com os valores documentados no README.
  - Garanta que os nomes antigos (`*_H`, `*_MIN`) **não** são mais lidos
    (defina-os no ambiente do teste e confirme que são ignorados).

## Arquivos a modificar

- `internal/config/config.go`
- `internal/config/config_test.go`
- `.env.example`
- `README.md` (tabela de variáveis + nota de breaking change)

## Definition of Done

- [x] Todas as variáveis de tempo usam sufixo `_SECONDS` e valores em segundos
- [x] Defaults documentados batem com os valores reais
- [x] `.env.example` e README atualizados, com nota de breaking change
- [x] Testes cobrindo os novos nomes e os defaults
- [x] Todos os testes passam

## Resolução

Renomeadas as quatro variáveis de tempo para o sufixo `_SECONDS`, com
defaults convertidos para o equivalente em segundos dos valores antigos
(sem mudar o comportamento padrão da aplicação):

| Antiga | Nova | Default antigo | Default novo |
|---|---|---|---|
| `UPLOAD_TOKEN_TTL_H` | `UPLOAD_TOKEN_TTL_SECONDS` | `6` (horas) | `21600` |
| `PLAY_TOKEN_MAX_TTL_H` | `PLAY_TOKEN_MAX_TTL_SECONDS` | `6` (horas) | `21600` |
| `UPLOAD_IDLE_TIMEOUT_MIN` | `UPLOAD_IDLE_TIMEOUT_SECONDS` | `10` (min) | `600` |
| `TRANSCODE_STUCK_MIN` | `TRANSCODE_STUCK_SECONDS` | `30` (min) | `1800` |

Mudanças:

- `internal/config/config.go`: lê as novas variáveis com `getEnvInt(..._SECONDS, <default em s>)`
  e monta os `time.Duration` com `time.Second * time.Duration(...)` (em vez de
  `time.Hour`/`time.Minute`). Os nomes antigos não são mais lidos —
  mudança incompatível intencional, sem aliases de compatibilidade.
- `internal/config/config_test.go`: três testes novos —
  `TestLoad_TimeVarsDefaultsAreInSeconds` (defaults batem com os valores
  antigos convertidos), `TestLoad_TimeVarsReadInSeconds` (valores
  customizados são lidos diretamente em segundos) e
  `TestLoad_OldTimeVarNamesAreIgnored` (nomes antigos definidos no ambiente
  não têm efeito — prevalecem os defaults novos).
- `.env.example` e `docker-compose.yml`: variáveis renomeadas com os novos
  valores-padrão em segundos.
- `README.md`: tabela de variáveis de ambiente atualizada + nota de
  **mudança incompatível** explicando a migração para quem tem instalação
  existente, e as três menções avulsas às variáveis antigas (TTL de
  reprodução, troubleshooting de upload/transcode travados) atualizadas
  para os novos nomes/unidades.

Issue #4 fechada pelo commit que acompanha esta tarefa.
