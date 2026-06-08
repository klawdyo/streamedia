// Package migrations embute os arquivos SQL de migração do banco no
// binário do servidor via go:embed. O runner do goose usa estes arquivos
// para aplicar migrations pendentes a cada inicialização — de forma
// idempotente (a tabela goose_db_version registra o que já foi aplicado).
package migrations

import "embed"

// FS contém todos os arquivos .sql do diretório de migrations, embutidos
// no binário em tempo de compilação — sem dependência de arquivos externos
// no ambiente de execução.
//
//go:embed *.sql
var FS embed.FS
