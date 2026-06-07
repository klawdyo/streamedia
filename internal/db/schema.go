package db

// schema define o DDL completo do banco de dados.
// Todas as declarações usam IF NOT EXISTS para ser idempotentes.
const schema = `
CREATE TABLE IF NOT EXISTS videos (
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

CREATE TABLE IF NOT EXISTS upload_tokens (
  token      TEXT PRIMARY KEY,
  video_id   TEXT NOT NULL UNIQUE,
  expires_at DATETIME NOT NULL,
  FOREIGN KEY (video_id) REFERENCES videos(video_id)
);

CREATE TABLE IF NOT EXISTS webhook_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  video_id   TEXT NOT NULL,
  event      TEXT NOT NULL,
  payload    TEXT,
  sent_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  success    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS playback_events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  video_id     TEXT NOT NULL,
  event_type   TEXT NOT NULL,
  resolution   INTEGER,
  user_agent   TEXT,
  os_family    TEXT,
  occurred_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- TODO(futuro): job de limpeza/retenção de eventos antigos, similar ao
-- killer de tokens expirados (T16). Esta tabela cresce sem limites por ora.

-- Projetos internos (issue #6, T32): cada app/ambiente que usa o Streamedia
-- (produção, staging, teste, ...) tem seu próprio namespace — slug usado
-- como diretório raiz dentro de MEDIA_DIR e chave mestra própria, usada
-- para emitir chaves de upload/leitura/admin escopadas (T33).
--
-- A chave mestra NUNCA é armazenada em texto puro — apenas seu hash
-- SHA-256 (mesmo princípio de não reter segredos em claro usado para os
-- demais segredos do sistema). O valor em texto puro é devolvido ao
-- cliente uma única vez, no momento da criação (ver CreateProject).
-- Nota (T33, issue #6): "videos" e "upload_tokens" ganham uma coluna
-- project_id (nullable, FK para projects) — adicionada via ALTER TABLE em
-- internal/db/db.go (ensureColumn), pois CREATE TABLE IF NOT EXISTS não
-- altera tabelas já existentes. project_id é NULL para vídeos/tokens
-- criados pelo fluxo legado (sem projeto), preservando compatibilidade.
CREATE TABLE IF NOT EXISTS projects (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT NOT NULL,
  slug            TEXT NOT NULL UNIQUE,
  root_dir        TEXT NOT NULL,
  master_key_hash TEXT NOT NULL,
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Variantes HLS geradas por vídeo (issue #5, T36): uma linha por
-- combinação (video_id, resolution), preenchida pelo worker FFmpeg (T11)
-- ao concluir cada variante — soma dos tamanhos dos segmentos .ts e a
-- contagem deles. Granularidade "por vídeo + por variante", não por chunk
-- de upload (os chunks do TUS são efêmeros — ver nota em
-- internal/models/storage.go): atende ao pedido da issue de "uma linha por
-- arquivo salvo" sem reter dados temporários sem valor analítico.
-- PRIMARY KEY composta: re-transcodificação substitui (UPSERT) a linha
-- existente em vez de duplicar.
CREATE TABLE IF NOT EXISTS video_renditions (
  video_id      TEXT    NOT NULL REFERENCES videos(video_id),
  resolution    INTEGER NOT NULL,
  size_bytes    INTEGER NOT NULL DEFAULT 0,
  segment_count INTEGER NOT NULL DEFAULT 0,
  updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (video_id, resolution)
);

CREATE INDEX IF NOT EXISTS idx_videos_status ON videos(status);
CREATE INDEX IF NOT EXISTS idx_videos_last_chunk ON videos(last_chunk_at);
CREATE INDEX IF NOT EXISTS idx_tokens_expires ON upload_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_playback_events_video ON playback_events(video_id);
CREATE INDEX IF NOT EXISTS idx_playback_events_occurred ON playback_events(occurred_at);
CREATE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug);
CREATE INDEX IF NOT EXISTS idx_video_renditions_video ON video_renditions(video_id);

CREATE TRIGGER IF NOT EXISTS videos_updated_at
AFTER UPDATE ON videos
FOR EACH ROW
BEGIN
  UPDATE videos SET updated_at = CURRENT_TIMESTAMP WHERE video_id = NEW.video_id;
END;
`
