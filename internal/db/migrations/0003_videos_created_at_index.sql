-- +goose Up
-- Dashboard administrativo: índice em videos(created_at).
--
-- O dashboard ordena vídeos por data de envio (ORDER BY created_at DESC em
-- /admin/videos) e agrega uploads por data/dia-da-semana/hora a partir de
-- created_at (internal/models/timeseries.go). Sem índice, essas consultas
-- fazem table scan. O índice mantém a listagem e os gráficos baratos conforme
-- a base de vídeos cresce.
CREATE INDEX IF NOT EXISTS idx_videos_created_at ON videos(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_videos_created_at;
