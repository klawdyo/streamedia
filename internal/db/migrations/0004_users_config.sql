-- +goose Up
-- Migration 0004: tabelas de usuários, roles e configurações dinâmicas.
--
-- users: usuários autenticados via Google OAuth. Email único.
-- user_roles: roles por usuário, com nível numérico (menor = mais poder).
--   DEV=1, ADMIN=2, ACL=3, MANAGER=4.
-- configurations: configurações dinâmicas em runtime, com tipo, validação,
--   agrupamento e flag de visibilidade (secretas nunca são lidas).
--
-- Bootstrapping: se a tabela users está vazia, o primeiro login Google
-- OAuth é aceito automaticamente com role 'dev'.

CREATE TABLE users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    email      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL DEFAULT '',
    picture    TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_roles (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('dev', 'admin', 'acl', 'manager')),
    level_num  INTEGER NOT NULL CHECK (level_num BETWEEN 1 AND 4),
    granted_by INTEGER REFERENCES users(id),
    granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, role)
);
CREATE INDEX idx_user_roles_user ON user_roles(user_id);

-- Configurações que podem ser alteradas em runtime sem restart.
-- visible=0 significa que o valor NUNCA é retornado em GET (ex: secrets) —
-- só aceita PUT (atualização cega). O frontend trata isso como campo "write-only".
CREATE TABLE configurations (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'string'
                CHECK (type IN ('string','number','boolean','duration_seconds','url','secret')),
    description TEXT NOT NULL DEFAULT '',
    group_key   TEXT NOT NULL DEFAULT '',
    validation  TEXT NOT NULL DEFAULT '',
    visible     INTEGER NOT NULL DEFAULT 1 CHECK (visible IN (0, 1)),
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Inserts dos defaults. Só insere se a linha não existir (idempotente via OR IGNORE).
-- Dessa forma, se o admin deletar uma config, ela NÃO renasce no próximo boot —
-- a funcionalidade usa o default do código Go (fallback).

INSERT OR IGNORE INTO configurations (key, value, type, description, group_key, validation, visible) VALUES
('paths.media_dir', '/media', 'string',
 'Diretório raiz onde os arquivos HLS transcodificados são armazenados. Cada vídeo fica em <media_dir>/<tag>/<video_id>/.',
 'paths', '', 1),

('paths.upload_tmp_dir', '/media/.uploads', 'string',
 'Diretório temporário onde os uploads TUS são gravados antes da validação pós-upload.',
 'paths', '', 1),

('session.ttl_seconds', '43200', 'duration_seconds',
 'Tempo de vida do cookie de sessão de navegador (streamedia_session), em segundos. Padrão: 43200 (12 horas).',
 'session', '^[1-9]\d*$', 1),

('upload.max_size_mb', '10', 'number',
 'Tamanho máximo de upload por vídeo, em megabytes. Vídeos acima disso são rejeitados na validação pós-upload.',
 'upload', '^[1-9]\d*$', 1),

('upload.idle_timeout', '600', 'duration_seconds',
 'Tempo máximo de inatividade de um upload TUS, em segundos. Uploads que ficam parados por mais que isso são mortos pelo job de limpeza.',
 'upload', '^[1-9]\d*$', 1),

('transcode.workers', '1', 'number',
 'Número de workers paralelos de transcodificação. Cada worker consome uma goroutine e um processo FFmpeg. Aumentar melhora o throughput de processamento mas consome mais CPU e memória.',
 'transcode', '^[1-9]\d*$', 1),

('transcode.queue_max', '50', 'number',
 'Tamanho máximo da fila de transcodificação. Quando a fila atinge esse limite, novos uploads são aceitos mas a transcodificação é pausada até haver vaga.',
 'transcode', '^[1-9]\d*$', 1),

('transcode.stuck_timeout', '1800', 'duration_seconds',
 'Tempo máximo que uma transcodificação pode ficar no estado "transcoding" antes de ser considerada travada e reenfileirada pelo job de requeue.',
 'transcode', '^[1-9]\d*$', 1),

('transcode.max_attempts', '3', 'number',
 'Número máximo de tentativas de transcodificação por vídeo. Após esse limite, o vídeo é marcado como failed_transcode.',
 'transcode', '^[1-9]\d*$', 1),

('transcode.keep_original', 'false', 'boolean',
 'Se true, mantém o arquivo original do upload após a transcodificação. Se false, deleta o original após gerar os segmentos HLS.',
 'transcode', '^(true|false)$', 1),

('token.upload_ttl', '1200', 'duration_seconds',
 'Tempo de vida do token de upload, em segundos. Tokens expirados são rejeitados pelo handler TUS e limpos pelo job de cleanup.',
 'token', '^[1-9]\d*$', 1),

('token.play_ttl', '3600', 'duration_seconds',
 'Tempo de vida do token de play, em segundos. Tokens expirados são rejeitados na validação do master playlist.',
 'token', '^[1-9]\d*$', 1),

('rate_limit.per_minute', '60', 'number',
 'Número máximo de requisições por minuto por IP. Aplicado globalmente a todas as rotas.',
 'rate_limit', '^[1-9]\d*$', 1),

('webhook.url', '', 'url',
 'URL global de webhook para notificações de eventos do pipeline. Se vazia, nenhum webhook é enviado. Pode ser sobrescrita por vídeo no upload/init.',
 'webhook', '^$|^https?://.*', 1),

('webhook.secret', '', 'secret',
 'Segredo compartilhado para assinatura HMAC-SHA256 dos webhooks. Write-only: nunca é retornado em GET, só aceita atualização via PUT.',
 'webhook', '', 0),

('discord.webhook_url', '', 'url',
 'URL do webhook do Discord para alertas operacionais internos (transcode travado, fila cheia, falhas consecutivas). Se vazia, o canal Discord é desabilitado.',
 'discord', '^$|^https://discord\.com/api/webhooks/.*', 1);

-- +goose Down
-- Rollback: remove na ordem inversa das dependências.
DROP TABLE IF EXISTS configurations;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS users;
