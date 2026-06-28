#!/bin/sh
# Entrypoint do container: garante a ownership correta dos diretórios
# persistidos ANTES de iniciar o servidor como usuário não-root.
#
# Por que isso existe: em produção (Coolify) os volumes /data e /media são
# BIND MOUNTS para um caminho do host (para aparecerem como diretório visível
# na aba de Persistent Storage). No host Linux esse diretório nasce como root,
# e o servidor roda como appuser (uid 10001) — sem este ajuste o SQLite falha
# ao gravar (error 14: unable to open database file).
#
# O container inicia como ROOT só para executar este chown; em seguida baixa o
# privilégio para appuser via su-exec e entrega o processo ao streamedia.
# Assim mantemos o servidor rodando como não-root, mas com os bind mounts
# graváveis.
set -e

# Garante que os diretórios existam e pertençam ao appuser.
mkdir -p /data /media /media/.uploads
chown -R appuser:appuser /data /media

# Baixa o privilégio e executa o servidor como appuser (substitui o shell,
# então o streamedia vira o PID 1 e recebe os sinais de shutdown).
exec su-exec appuser:appuser streamedia "$@"
