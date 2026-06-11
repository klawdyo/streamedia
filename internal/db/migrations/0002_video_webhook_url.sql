-- +goose Up
-- Issue #20: URL de webhook customizada por vídeo.
--
-- Adiciona a coluna `webhook_url` em `videos`. Quando preenchida (com uma URL
-- HTTPS válida, informada no POST /api/upload/init), os webhooks daquele vídeo
-- são enviados para essa URL em vez da WEBHOOK_URL global. Vazia ('') significa
-- "usar a WEBHOOK_URL global" — o comportamento histórico. NOT NULL DEFAULT ''
-- garante que vídeos antigos (e inserts internos que não a informam) continuem
-- válidos sem migração de dados.
ALTER TABLE videos ADD COLUMN webhook_url TEXT NOT NULL DEFAULT '';

-- +goose Down
-- Rollback: remove a coluna (SQLite >= 3.35 suporta DROP COLUMN).
ALTER TABLE videos DROP COLUMN webhook_url;
