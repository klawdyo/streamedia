package docs

// openAPISpec monta a especificação OpenAPI 3.0 da API Streamedia como um
// literal Go (mapas/slices), serializado para JSON pelo Handler.
//
// Optamos por escrever a spec manualmente (em vez de gerar via anotações
// `swaggo/swag` nos handlers) porque a API é pequena e estável o bastante
// para que a manutenção manual seja mais simples do que manter comentários
// de anotação espalhados por handlers de pacotes diferentes — e evita um
// passo de geração de código (`swag init`) no fluxo de build/CI.
//
// Cobre as rotas listadas em T30: upload (init + protocolo TUS), status,
// serving HLS, rotas administrativas (videos, queue, stats, projects — T35)
// e referencia /metrics em alto nível (formato Prometheus, não é uma rota
// JSON).
func openAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Streamedia API",
			"description": "Serviço de upload, transcodificação e entrega de vídeo em HLS.",
			"version":     "1.0.0",
		},
		"tags": []map[string]any{
			{"name": "upload", "description": "Envio de arquivos de vídeo (protocolo TUS resumível)"},
			{"name": "status", "description": "Consulta de status de processamento de vídeos"},
			{"name": "serving", "description": "Entrega de conteúdo HLS (master playlist, playlists e segmentos)"},
			{"name": "admin", "description": "Rotas administrativas (protegidas por token de admin)"},
			{"name": "observability", "description": "Métricas e estatísticas operacionais"},
		},
		"paths": map[string]any{
			"/upload/init": map[string]any{
				"post": map[string]any{
					"tags":        []string{"upload"},
					"summary":     "Inicia um upload",
					"description": "Valida o HMAC da requisição e gera um token de upload de curta duração, usado em seguida para autenticar a sessão TUS em /files.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"filename":   map[string]any{"type": "string", "example": "video.mp4"},
										"size":       map[string]any{"type": "integer", "format": "int64", "example": 10485760},
										"timestamp":  map[string]any{"type": "integer", "format": "int64", "description": "Unix timestamp usado na assinatura HMAC"},
										"signature":  map[string]any{"type": "string", "description": "Assinatura HMAC-SHA256 da requisição"},
									},
									"required": []string{"filename", "size", "timestamp", "signature"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Token de upload gerado",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"video_id":   map[string]any{"type": "string"},
											"upload_url": map[string]any{"type": "string"},
											"token":      map[string]any{"type": "string"},
											"expires_at": map[string]any{"type": "string", "format": "date-time"},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Requisição inválida"},
						"401": map[string]any{"description": "Assinatura HMAC inválida ou expirada"},
					},
				},
			},
			"/files": map[string]any{
				"post": map[string]any{
					"tags":        []string{"upload"},
					"summary":     "Cria uma sessão de upload TUS",
					"description": "Endpoint do protocolo TUS (resumable upload protocol — https://tus.io). O Streamedia implementa o servidor TUS via biblioteca tusd; os detalhes do protocolo (cabeçalhos Tus-Resumable, Upload-Offset, Upload-Length, etc.) seguem a especificação oficial. A sessão é autenticada pelo token gerado em /upload/init.",
					"responses": map[string]any{
						"201": map[string]any{"description": "Sessão de upload criada"},
						"401": map[string]any{"description": "Token de upload ausente, inválido ou expirado"},
					},
				},
			},
			"/files/{video_id}": map[string]any{
				"parameters": []map[string]any{
					{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"head": map[string]any{
					"tags":        []string{"upload"},
					"summary":     "Consulta o offset de um upload TUS em andamento",
					"description": "Endpoint do protocolo TUS para retomar uploads — devolve o offset atual via cabeçalho Upload-Offset.",
					"responses": map[string]any{"200": map[string]any{"description": "Offset atual do upload"}},
				},
				"patch": map[string]any{
					"tags":        []string{"upload"},
					"summary":     "Envia um chunk de dados (protocolo TUS)",
					"description": "Anexa bytes ao upload em andamento, a partir do offset informado em Upload-Offset.",
					"responses": map[string]any{
						"204": map[string]any{"description": "Chunk recebido; upload pode continuar ou ter concluído"},
						"409": map[string]any{"description": "Conflito de offset"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"upload"},
					"summary":     "Cancela um upload TUS em andamento",
					"responses":   map[string]any{"204": map[string]any{"description": "Upload cancelado"}},
				},
			},
			"/api/status/{video_id}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"status"},
					"summary":     "Consulta o status de processamento de um vídeo",
					"description": "Devolve o estado atual do vídeo na máquina de estados (uploading, processing, ready, failed, etc.) e metadados associados. Requer autenticação HMAC.",
					"parameters": []map[string]any{
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Status do vídeo",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"video_id": map[string]any{"type": "string"},
											"status":   map[string]any{"type": "string", "example": "ready"},
											"progress": map[string]any{"type": "number", "format": "float"},
										},
									},
								},
							},
						},
						"401": map[string]any{"description": "Assinatura HMAC inválida"},
						"404": map[string]any{"description": "Vídeo não encontrado"},
					},
				},
			},
			"/videos/{video_id}/master.m3u8": map[string]any{
				"get": map[string]any{
					"tags":        []string{"serving"},
					"summary":     "Master playlist HLS de um vídeo",
					"description": "Devolve o arquivo master.m3u8 com as variantes de resolução disponíveis para o vídeo. Requer token de leitura autenticado (assinado por HMAC, escopado ao vídeo).",
					"parameters": []map[string]any{
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string"}, "description": "Token de leitura assinado, escopado a este vídeo"},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Master playlist HLS", "content": map[string]any{"application/vnd.apple.mpegurl": map[string]any{}}},
						"401": map[string]any{"description": "Token ausente, inválido ou expirado"},
						"404": map[string]any{"description": "Vídeo não encontrado ou ainda não processado"},
					},
				},
			},
			"/videos/{video_id}/{resolution}/playlist.m3u8": map[string]any{
				"get": map[string]any{
					"tags":        []string{"serving"},
					"summary":     "Playlist HLS de uma resolução específica",
					"parameters": []map[string]any{
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "resolution", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "example": "720p"},
						{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Playlist HLS da resolução", "content": map[string]any{"application/vnd.apple.mpegurl": map[string]any{}}},
						"401": map[string]any{"description": "Token ausente, inválido ou expirado"},
						"404": map[string]any{"description": "Playlist não encontrada"},
					},
				},
			},
			"/videos/{video_id}/{resolution}/{segment}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"serving"},
					"summary":     "Segmento de vídeo HLS (.ts)",
					"description": "Entrega um segmento individual da resolução solicitada, servido estaticamente após autenticação por token.",
					"parameters": []map[string]any{
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "resolution", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "example": "720p"},
						{"name": "segment", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "example": "segment000.ts"},
						{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Segmento de vídeo", "content": map[string]any{"video/mp2t": map[string]any{}}},
						"401": map[string]any{"description": "Token ausente, inválido ou expirado"},
						"404": map[string]any{"description": "Segmento não encontrado"},
					},
				},
			},
			"/admin/videos": map[string]any{
				"get": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Lista vídeos cadastrados",
					"description": "Lista paginada de vídeos com seus status. Requer token de administração (cabeçalho Authorization: Bearer <ADMIN_TOKEN>).",
					"security":    []map[string]any{{"adminToken": []string{}}},
					"responses": map[string]any{
						"200": map[string]any{"description": "Lista de vídeos"},
						"401": map[string]any{"description": "Token de administração ausente ou inválido"},
					},
				},
			},
			"/admin/queue": map[string]any{
				"get": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Inspeciona a fila de transcodificação",
					"description": "Devolve o tamanho atual da fila e os jobs em processamento. Requer token de administração.",
					"security":    []map[string]any{{"adminToken": []string{}}},
					"responses": map[string]any{
						"200": map[string]any{"description": "Estado da fila de transcodificação"},
						"401": map[string]any{"description": "Token de administração ausente ou inválido"},
					},
				},
			},
			"/admin/stats": map[string]any{
				"get": map[string]any{
					"tags":        []string{"admin", "observability"},
					"summary":     "Estatísticas agregadas de uso",
					"description": "Agrega eventos brutos de reprodução/upload (T26/T27) em totais por tipo de evento, contagens por resolução, sistema operacional e dia da semana. Requer token de administração.",
					"security":    []map[string]any{{"adminToken": []string{}}},
					"parameters": []map[string]any{
						{"name": "video_id", "in": "query", "required": false, "schema": map[string]any{"type": "string"}, "description": "Restringe a agregação a um único vídeo; sem este parâmetro, a visão é global"},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Estatísticas agregadas",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"video_id":        map[string]any{"type": "string", "nullable": true},
											"totals":          map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
											"by_resolution":   map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
											"by_os":           map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
											"by_day_of_week":  map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
										},
									},
								},
							},
						},
						"401": map[string]any{"description": "Token de administração ausente ou inválido"},
						"404": map[string]any{"description": "video_id informado não corresponde a um vídeo existente"},
					},
				},
			},
			"/admin/projects": map[string]any{
				"post": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Cria um projeto interno",
					"description": "Cria um namespace isolado (diretório de armazenamento próprio + chave mestra) — issue #6, T32/T35. Operação de super-admin: cria os próprios projetos e suas chaves mestras, então exige o ADMIN_TOKEN global (uma chave mestra de projeto não autentica aqui).",
					"security":    []map[string]any{{"adminToken": []string{}}},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":       "object",
									"properties": map[string]any{"name": map[string]any{"type": "string", "example": "Trip Produção"}},
									"required":   []string{"name"},
								},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{
							"description": "Projeto criado — master_key é devolvida em texto puro apenas nesta resposta",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"id":         map[string]any{"type": "integer", "format": "int64"},
											"name":       map[string]any{"type": "string"},
											"slug":       map[string]any{"type": "string"},
											"root_dir":   map[string]any{"type": "string"},
											"master_key": map[string]any{"type": "string", "description": "Chave mestra em texto puro — única vez em que é exposta; o servidor persiste apenas seu hash"},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Corpo inválido ou campo 'name' ausente"},
						"401": map[string]any{"description": "Token de administração ausente ou inválido"},
						"403": map[string]any{"description": "Autenticado com chave mestra de projeto — apenas o ADMIN_TOKEN global pode criar projetos"},
					},
				},
				"get": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Lista projetos cadastrados",
					"description": "Lista todos os projetos sem expor master_key/hash. Operação de super-admin (mesma restrição do POST).",
					"security":    []map[string]any{{"adminToken": []string{}}},
					"responses": map[string]any{
						"200": map[string]any{"description": "Lista de projetos"},
						"401": map[string]any{"description": "Token de administração ausente ou inválido"},
						"403": map[string]any{"description": "Autenticado com chave mestra de projeto — apenas o ADMIN_TOKEN global enxerga o catálogo completo"},
					},
				},
			},
			"/admin/projects/{slug}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Detalhe de um projeto",
					"description": "Devolve os dados públicos de um projeto pelo slug (sem master_key/hash). Operação de super-admin.",
					"security":    []map[string]any{{"adminToken": []string{}}},
					"parameters": []map[string]any{
						{"name": "slug", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "description": "Slug do projeto"},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Dados do projeto"},
						"401": map[string]any{"description": "Token de administração ausente ou inválido"},
						"403": map[string]any{"description": "Autenticado com chave mestra de projeto — apenas o ADMIN_TOKEN global pode consultar projetos por este endpoint"},
						"404": map[string]any{"description": "Slug não corresponde a nenhum projeto"},
					},
				},
			},
			"/admin/projects/{slug}/upload-tokens": map[string]any{
				"post": map[string]any{
					"tags":        []string{"admin", "upload"},
					"summary":     "Emite um token de upload escopado para um vídeo recém-gerado",
					"description": "Troca a chave mestra do projeto por um token de upload de curta duração para um video_id gerado pelo servidor — equivalente a POST /upload/init no fluxo escopado a projeto (T33), sem o cliente precisar gerar o UUID previamente. Autenticação própria via X-Project-Key (NÃO usa Authorization/ADMIN_TOKEN); o {slug} do path deve corresponder ao projeto resolvido pela chave.",
					"security":    []map[string]any{{"projectKey": []string{}}},
					"parameters": []map[string]any{
						{"name": "slug", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "description": "Slug do projeto — deve corresponder ao projeto da chave mestra apresentada"},
					},
					"requestBody": map[string]any{
						"required": false,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":       "object",
									"properties": map[string]any{"declared_size_bytes": map[string]any{"type": "integer", "format": "int64", "example": 52428800}},
								},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{
							"description": "Token de upload emitido (TTL curto — UPLOAD_TOKEN_SCOPED_TTL_SECONDS, padrão 1200s)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"video_id":   map[string]any{"type": "string", "format": "uuid"},
											"upload_url": map[string]any{"type": "string", "format": "uri"},
											"token":      map[string]any{"type": "string"},
											"expires_at": map[string]any{"type": "string", "format": "date-time"},
										},
									},
								},
							},
						},
						"401": map[string]any{"description": "Header X-Project-Key ausente ou chave inválida"},
						"403": map[string]any{"description": "A chave informada não pertence ao projeto identificado pelo {slug}"},
					},
				},
			},
			"/metrics": map[string]any{
				"get": map[string]any{
					"tags":        []string{"observability"},
					"summary":     "Métricas operacionais (formato Prometheus)",
					"description": "Expõe métricas operacionais (taxa de requisições, latência, tamanho de filas, gauges de negócio) no formato de texto Prometheus, coletável por scrapers OpenTelemetry/Prometheus. Não é uma rota JSON — por isso não tem um schema de resposta detalhado nesta especificação OpenAPI.",
					"responses": map[string]any{
						"200": map[string]any{"description": "Métricas no formato de exposição do Prometheus", "content": map[string]any{"text/plain": map[string]any{}}},
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"adminToken": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"description":  "Token de administração configurado via variável de ambiente ADMIN_TOKEN. Aceita também a chave mestra de um projeto (escopo restrito aos seus próprios vídeos — issue #6, T33), exceto nas rotas de gerenciamento de projetos, que exigem o token global.",
				},
				"projectKey": map[string]any{
					"type":        "apiKey",
					"in":          "header",
					"name":        "X-Project-Key",
					"description": "Chave mestra do projeto em texto puro (issue #6, T33/T35). O servidor calcula seu hash SHA-256 e resolve o projeto correspondente — nunca persiste ou recupera a chave em texto puro. Mesmo header usado por POST /upload/init no fluxo escopado a projeto.",
				},
			},
		},
	}
}
