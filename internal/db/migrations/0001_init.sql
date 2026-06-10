-- +goose Up
-- Migration inicial: schema completo do Streamedia.
--
-- Modelo de namespace por TAG (substitui o antigo modelo de "projects" com
-- chave mestra): cada vídeo carrega uma `tag` (slug) que serve apenas como
-- namespace organizacional e de armazenamento (<MEDIA_DIR>/<tag>/<video_id>).
-- Não há credencial por tag — toda a gestão é feita com o ROOT_TOKEN único.
--
-- Os tokens efêmeros de upload e de play vivem juntos em `access_tokens`,
-- distinguidos pela coluna `purpose` ('upload' | 'play'): são strings
-- aleatórias validadas por lookup (sem HMAC, sem secret de assinatura).

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
    -- tag: namespace do vídeo. NOT NULL com DEFAULT 'default' — a API
    -- (/api/upload/init) sempre exige uma tag explícita não-vazia; o default
    -- só serve para inserts internos/diretos que não a informam.
    tag                 TEXT NOT NULL DEFAULT 'default',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tokens efêmeros de acesso (upload e play). `purpose` separa os dois
-- propósitos: um token de play nunca autoriza upload e vice-versa. A
-- unicidade é por (video_id, purpose) — no máximo um token ativo de cada
-- propósito por vídeo.
CREATE TABLE access_tokens (
    token       TEXT PRIMARY KEY,
    video_id    TEXT NOT NULL,
    purpose     TEXT NOT NULL,
    expires_at  DATETIME NOT NULL,
    UNIQUE (video_id, purpose),
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

CREATE TABLE playback_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id     TEXT NOT NULL,
    event_type   TEXT NOT NULL,
    resolution   INTEGER,
    user_agent   TEXT,
    os_family    TEXT,
    occurred_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE video_renditions (
    video_id      TEXT    NOT NULL REFERENCES videos(video_id),
    resolution    INTEGER NOT NULL,
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    segment_count INTEGER NOT NULL DEFAULT 0,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (video_id, resolution)
);

CREATE INDEX idx_videos_status ON videos(status);
CREATE INDEX idx_videos_last_chunk ON videos(last_chunk_at);
CREATE INDEX idx_videos_tag ON videos(tag);
CREATE INDEX idx_access_tokens_expires ON access_tokens(expires_at);
CREATE INDEX idx_playback_events_video ON playback_events(video_id);
CREATE INDEX idx_playback_events_occurred ON playback_events(occurred_at);
CREATE INDEX idx_video_renditions_video ON video_renditions(video_id);

-- +goose StatementBegin
CREATE TRIGGER videos_updated_at
AFTER UPDATE ON videos
FOR EACH ROW
BEGIN
    UPDATE videos SET updated_at = CURRENT_TIMESTAMP WHERE video_id = NEW.video_id;
END;
-- +goose StatementEnd

-- +goose Down
-- Rollback: desfaz tudo na ordem inversa, respeitando foreign keys
-- e removendo entidades dependentes antes das independentes.

DROP TRIGGER IF EXISTS videos_updated_at;
DROP INDEX IF EXISTS idx_video_renditions_video;
DROP INDEX IF EXISTS idx_playback_events_occurred;
DROP INDEX IF EXISTS idx_playback_events_video;
DROP INDEX IF EXISTS idx_access_tokens_expires;
DROP INDEX IF EXISTS idx_videos_tag;
DROP INDEX IF EXISTS idx_videos_last_chunk;
DROP INDEX IF EXISTS idx_videos_status;
DROP TABLE IF EXISTS video_renditions;
DROP TABLE IF EXISTS playback_events;
DROP TABLE IF EXISTS access_tokens;
DROP TABLE IF EXISTS videos;
DROP TABLE IF EXISTS webhook_log;
