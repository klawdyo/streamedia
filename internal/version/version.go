// Package version expõe as informações de build do binário — nome, versão e
// commit — injetadas via -ldflags no go build.
//
// Os valores padrão ("0.0.0-dev", "unknown") são usados em desenvolvimento
// (go run / go test). Em builds oficiais (Docker, CI), os valores reais
// são injetados pelo linker:
//
//	go build -ldflags="
//	  -X github.com/klawdyo/streamedia/internal/version.Version=0.35.0
//	  -X github.com/klawdyo/streamedia/internal/version.Commit=$(git rev-parse --short HEAD)
//	" -o mediaserver ./cmd/server
//
// A rota GET /api expõe a versão, o ambiente e o status no envelope padrão.
package version

// Versão semântica do binário, injetada via -ldflags.
// Valor padrão "0.0.0-dev" aparece em builds de desenvolvimento.
var Version = "0.0.0-dev"

// Commit é o hash curto do commit Git no momento do build, injetado via
// -ldflags. É embutido no binário para fins de depuração, mas NÃO é exposto
// na rota pública GET /api — evitamos divulgar informação de build a quem
// faz reconhecimento do serviço. Continua disponível internamente (ex. logs).
var Commit = "unknown"

// VersionInfo agrupa as informações expostas pela rota GET /api.
// O ambiente (production/development) vem da configuração de runtime (ENV),
// não do build — por isso é recebido como parâmetro em Get, e não declarado
// como variável injetada por -ldflags.
type VersionInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
	Status      string `json:"status"`
}

// Get devolve as informações expostas pela rota /api: a versão vem do valor
// injetado via -ldflags (ou o default em desenvolvimento) e o ambiente é
// fornecido pelo chamador a partir da configuração (variável ENV).
func Get(environment string) VersionInfo {
	return VersionInfo{
		Name:        "Streamedia",
		Version:     Version,
		Environment: environment,
		Status:      "ok",
	}
}
