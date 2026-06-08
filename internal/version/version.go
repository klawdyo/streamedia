// Package version expõe as informações de build do binário — nome, versão,
// commit e timestamp — injetadas via -ldflags no go build.
//
// Os valores padrão ("0.0.0-dev", "unknown") são usados em desenvolvimento
// (go run / go test). Em builds oficiais (Docker, CI), os valores reais
// são injetados pelo linker:
//
//	go build -ldflags="
//	  -X github.com/klawdyo/streamedia/internal/version.Version=0.35.0
//	  -X github.com/klawdyo/streamedia/internal/version.Commit=$(git rev-parse --short HEAD)
//	  -X github.com/klawdyo/streamedia/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
//	" -o mediaserver ./cmd/server
//
// A rota GET /api expõe essas informações no envelope padrão da API.
package version

// Versão semântica do binário, injetada via -ldflags.
// Valor padrão "0.0.0-dev" aparece em builds de desenvolvimento.
var Version = "0.0.0-dev"

// Hash curto do commit Git no momento do build.
var Commit = "unknown"

// Timestamp UTC de quando o binário foi compilado.
var BuildTime = "unknown"

// VersionInfo agrupa as informações de build expostas pela rota GET /api.
type VersionInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
	Status    string `json:"status"`
}

// Get devolve as informações de build, usando os valores injetados via
// -ldflags (ou os defaults em desenvolvimento).
func Get() VersionInfo {
	return VersionInfo{
		Name:      "Streamedia",
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		Status:    "ok",
	}
}
