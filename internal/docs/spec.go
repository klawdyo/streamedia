package docs

// openAPISpec monta a especificação OpenAPI 3.0 da API Streamedia como um
// literal Go (mapas/slices), serializado para JSON pelo Handler.
//
// A spec é escrita manualmente (em vez de gerada via anotações) porque a API é
// pequena e estável o bastante para que a manutenção manual seja mais simples
// e evite um passo de geração de código no build/CI.
//
// Modelo de autenticação: uma credencial única de gestão (ROOT_TOKEN, em
// Authorization: Bearer) protege /api/upload/init, /api/play/init,
// /api/status e /admin/*. Os tokens efêmeros de upload (Upload-Token no TUS) e
// de play (query ?token=) são emitidos por essas rotas e validados por lookup.
func openAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Streamedia API",
			"description": "Serviço de upload, transcodificação e entrega de vídeo em HLS.",
			"version":     "1.0.0",
		},
		"tags": []map[string]any{
			{"name": "upload", "description": "Início de upload e protocolo TUS resumível"},
			{"name": "play", "description": "Emissão de URL assinada e entrega de conteúdo HLS"},
			{"name": "status", "description": "Consulta de status de processamento de vídeos"},
			{"name": "admin", "description": "Rotas administrativas (protegidas pelo ROOT_TOKEN)"},
			{"name": "observability", "description": "Versão, métricas e estatísticas operacionais"},
		},
		"paths": map[string]any{
			"/api/upload/init": map[string]any{
				"post": map[string]any{
					"tags":        []string{"upload"},
					"summary":     "Inicia um upload",
					"description": "Registra o vídeo no namespace (tag) informado e gera um token de upload efêmero, usado em seguida para autenticar a sessão TUS em /files. Se video_id for omitido, o servidor gera um UUID v7.",
					"security":    []map[string]any{{"rootToken": []string{}}},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"tag":                 map[string]any{"type": "string", "description": "Namespace organizacional do vídeo (normalizado para slug)", "example": "minha-tag"},
										"video_id":            map[string]any{"type": "string", "description": "UUID do vídeo (opcional — se omitido, o servidor gera UUID v7)"},
										"declared_size_bytes": map[string]any{"type": "integer", "format": "int64", "description": "Tamanho declarado do arquivo em bytes", "example": 10485760},
									},
									"required": []string{"tag", "declared_size_bytes"},
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
											"tag":        map[string]any{"type": "string"},
											"upload_url": map[string]any{"type": "string"},
											"token":      map[string]any{"type": "string"},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "JSON inválido, tag ausente ou video_id não é UUID"},
						"401": map[string]any{"description": "ROOT_TOKEN ausente ou inválido"},
						"409": map[string]any{"description": "video_id já existe"},
						"413": map[string]any{"description": "declared_size_bytes acima do limite"},
					},
				},
			},
			"/files": map[string]any{
				"post": map[string]any{
					"tags":        []string{"upload"},
					"summary":     "Cria uma sessão de upload TUS",
					"description": "Endpoint do protocolo TUS (https://tus.io), implementado via tusd. A sessão é autenticada pelo token efêmero gerado em /api/upload/init, enviado no header Upload-Token.",
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
					"responses":   map[string]any{"200": map[string]any{"description": "Offset atual do upload"}},
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
					"tags":      []string{"upload"},
					"summary":   "Cancela um upload TUS em andamento",
					"responses": map[string]any{"204": map[string]any{"description": "Upload cancelado"}},
				},
			},
			"/api/play/init": map[string]any{
				"post": map[string]any{
					"tags":        []string{"play"},
					"summary":     "Emite uma URL de reprodução assinada",
					"description": "O backend principal (que já autorizou o usuário) troca o ROOT_TOKEN por uma URL assinada e de curta duração do master playlist. O Streamedia gera um token de play efêmero (lookup no banco, TTL PLAY_TOKEN_TTL) e devolve a play_url junto com a lista de resoluções (variantes HLS) disponíveis para o vídeo.",
					"security":    []map[string]any{{"rootToken": []string{}}},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":       "object",
									"properties": map[string]any{"video_id": map[string]any{"type": "string", "format": "uuid"}},
									"required":   []string{"video_id"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "URL de reprodução assinada",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"video_id":   map[string]any{"type": "string"},
											"tag":        map[string]any{"type": "string"},
											"play_url":   map[string]any{"type": "string", "format": "uri"},
											"token":      map[string]any{"type": "string"},
											"expires_at": map[string]any{"type": "string", "format": "date-time"},
											"resolutions": map[string]any{
												"type":        "array",
												"items":       map[string]any{"type": "integer"},
												"description": "Resoluções (alturas) das variantes HLS disponíveis, ordenadas asc. Ex.: [480, 720, 1080].",
											},
										},
									},
								},
							},
						},
						"401": map[string]any{"description": "ROOT_TOKEN ausente ou inválido"},
						"404": map[string]any{"description": "Vídeo não encontrado"},
						"409": map[string]any{"description": "Vídeo não está pronto (status != ready)"},
					},
				},
			},
			"/api/events": map[string]any{
				"get": map[string]any{
					"tags":        []string{"play"},
					"summary":     "Stream de eventos do vídeo (Server-Sent Events)",
					"description": "Stream SSE (text/event-stream) com as notificações do pipeline de um vídeo (processing, ready, failed) ao vivo — os mesmos dados do webhook. Escopado por video_id e autenticado pelo token de upload do vídeo (na query, pois EventSource não envia cabeçalhos). Cada evento chega como 'event: <nome>' + 'data: <json>'. Sem buffer/replay.",
					"parameters": []map[string]any{
						{"name": "video_id", "in": "query", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
						{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string"}, "description": "Token de upload do vídeo (o 'token' devolvido por /api/upload/init)"},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Stream SSE aberto (text/event-stream)",
							"content":     map[string]any{"text/event-stream": map[string]any{}},
						},
						"400": map[string]any{"description": "Faltam video_id ou token"},
						"401": map[string]any{"description": "Token inválido, expirado ou de outro vídeo"},
					},
				},
			},
			"/api/status/{video_id}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"status"},
					"summary":     "Consulta o status de processamento de um vídeo",
					"description": "Devolve o estado atual do vídeo na máquina de estados (pending_upload, uploading, transcoding, ready, failed_*) e metadados associados.",
					"security":    []map[string]any{{"rootToken": []string{}}},
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
										},
									},
								},
							},
						},
						"401": map[string]any{"description": "ROOT_TOKEN ausente ou inválido"},
						"404": map[string]any{"description": "Vídeo não encontrado"},
					},
				},
			},
			"/video/{tag}/{video_id}.m3u8": map[string]any{
				"get": map[string]any{
					"tags":        []string{"play"},
					"summary":     "Master playlist HLS (dinâmico)",
					"description": "Devolve o master.m3u8 com as variantes de resolução, reescrevendo os caminhos internos. Requer o token de play na query (validado por lookup no banco). O caminho real no disco fica escondido.",
					"parameters": []map[string]any{
						{"name": "tag", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string"}, "description": "Token de play emitido por /api/play/init"},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Master playlist HLS", "content": map[string]any{"application/vnd.apple.mpegurl": map[string]any{}}},
						"401": map[string]any{"description": "Token ausente, inválido ou expirado"},
						"404": map[string]any{"description": "Vídeo não encontrado ou ainda não processado"},
					},
				},
			},
			"/video/{tag}/{video_id}/{resolution}/playlist.m3u8": map[string]any{
				"get": map[string]any{
					"tags":    []string{"play"},
					"summary": "Playlist HLS de uma resolução (estática, pública)",
					"parameters": []map[string]any{
						{"name": "tag", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "resolution", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "example": "720"},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Playlist HLS da resolução", "content": map[string]any{"application/vnd.apple.mpegurl": map[string]any{}}},
						"404": map[string]any{"description": "Playlist não encontrada"},
					},
				},
			},
			"/video/{tag}/{video_id}/{resolution}/{segment}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"play"},
					"summary":     "Segmento de vídeo HLS (.ts) — estático, público",
					"description": "Entrega um segmento individual da resolução solicitada. Os nomes opacos contidos no master playlist funcionam como a 'chave' de acesso.",
					"parameters": []map[string]any{
						{"name": "tag", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
						{"name": "resolution", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "example": "720"},
						{"name": "segment", "in": "path", "required": true, "schema": map[string]any{"type": "string"}, "example": "0.ts"},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Segmento de vídeo", "content": map[string]any{"video/mp2t": map[string]any{}}},
						"404": map[string]any{"description": "Segmento não encontrado"},
					},
				},
			},
			"/admin/videos": map[string]any{
				"get": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Lista vídeos cadastrados",
					"description": "Lista paginada de vídeos com seus status. Aceita os query params opcionais status, tag, limit e offset.",
					"security":    []map[string]any{{"rootToken": []string{}}},
					"parameters": []map[string]any{
						{"name": "status", "in": "query", "required": false, "schema": map[string]any{"type": "string"}},
						{"name": "tag", "in": "query", "required": false, "schema": map[string]any{"type": "string"}},
						{"name": "limit", "in": "query", "required": false, "schema": map[string]any{"type": "integer"}},
						{"name": "offset", "in": "query", "required": false, "schema": map[string]any{"type": "integer"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Lista de vídeos"},
						"401": map[string]any{"description": "ROOT_TOKEN ausente ou inválido"},
					},
				},
			},
			"/admin/videos/{video_id}": map[string]any{
				"delete": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Apaga um vídeo",
					"description": "Remove as linhas do vídeo no banco (tokens, variantes, eventos e o próprio vídeo) e o diretório de arquivos no disco.",
					"security":    []map[string]any{{"rootToken": []string{}}},
					"parameters": []map[string]any{
						{"name": "video_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					},
					"responses": map[string]any{
						"200": map[string]any{"description": "Vídeo apagado"},
						"401": map[string]any{"description": "ROOT_TOKEN ausente ou inválido"},
						"404": map[string]any{"description": "Vídeo não encontrado"},
					},
				},
			},
			"/admin/queue": map[string]any{
				"get": map[string]any{
					"tags":        []string{"admin"},
					"summary":     "Inspeciona a fila de transcodificação",
					"description": "Devolve o tamanho atual da fila e o número de workers.",
					"security":    []map[string]any{{"rootToken": []string{}}},
					"responses": map[string]any{
						"200": map[string]any{"description": "Estado da fila de transcodificação"},
						"401": map[string]any{"description": "ROOT_TOKEN ausente ou inválido"},
					},
				},
			},
			"/admin/stats": map[string]any{
				"get": map[string]any{
					"tags":        []string{"admin", "observability"},
					"summary":     "Estatísticas agregadas de uso",
					"description": "Agrega eventos de reprodução/upload em totais por tipo de evento, resolução, sistema operacional e dia da semana — e, na visão global (sem ?video_id=), também armazenamento e fila (bytes totais, duração, vídeos por status, fila pendente).",
					"security":    []map[string]any{{"rootToken": []string{}}},
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
											"video_id":       map[string]any{"type": "string", "nullable": true},
											"totals":         map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
											"by_resolution":  map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
											"by_os":          map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
											"by_day_of_week": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
											"storage": map[string]any{
												"type":        "object",
												"nullable":    true,
												"description": "Visão GLOBAL de armazenamento e fila; presente apenas quando ?video_id= não é informado.",
												"properties": map[string]any{
													"total_bytes":            map[string]any{"type": "integer", "format": "int64"},
													"total_duration_seconds": map[string]any{"type": "integer", "format": "int64"},
													"videos_by_status":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}},
													"queue_pending":          map[string]any{"type": "integer"},
												},
											},
										},
									},
								},
							},
						},
						"401": map[string]any{"description": "ROOT_TOKEN ausente ou inválido"},
						"404": map[string]any{"description": "video_id informado não corresponde a um vídeo existente"},
					},
				},
			},
			"/api": map[string]any{
				"get": map[string]any{
					"tags":        []string{"observability"},
					"summary":     "Versão e status da API",
					"description": "Retorna nome, versão semântica, ambiente de execução (ENV) e status da API. Rota pública sem autenticação, com rate limiting de 10 req/min.",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Informações de versão no envelope padrão",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"name":        map[string]any{"type": "string", "example": "Streamedia"},
											"version":     map[string]any{"type": "string", "example": "0.42.0"},
											"environment": map[string]any{"type": "string", "example": "production"},
											"status":      map[string]any{"type": "string", "example": "ok"},
										},
									},
								},
							},
						},
						"429": map[string]any{"description": "Rate limit excedido (10 req/min)"},
					},
				},
			},
			"/metrics": map[string]any{
				"get": map[string]any{
					"tags":        []string{"observability"},
					"summary":     "Métricas operacionais (formato Prometheus)",
					"description": "Expõe métricas operacionais no formato de texto Prometheus, coletável por scrapers OpenTelemetry/Prometheus. Não é uma rota JSON.",
					"responses": map[string]any{
						"200": map[string]any{"description": "Métricas no formato de exposição do Prometheus", "content": map[string]any{"text/plain": map[string]any{}}},
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"rootToken": map[string]any{
					"type":        "http",
					"scheme":      "bearer",
					"description": "Credencial única de gestão (ROOT_TOKEN), configurada via variável de ambiente. Protege /api/upload/init, /api/play/init, /api/status e /admin/*.",
				},
			},
		},
	}
}
