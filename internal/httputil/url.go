// Package httputil reúne funções auxiliares de HTTP compartilhadas entre
// vários handlers — resolução de scheme/host a partir de headers de proxy
// e construção de URLs públicas.
//
// Centraliza a lógica antes duplicada entre /upload/init e
// /admin/projects/{slug}/upload-tokens: ambas constroem a mesma forma de
// URL (<scheme>://<host>/files/<video_id>) a partir dos mesmos headers.
package httputil

import (
	"fmt"
	"net/http"
)

// PublicUploadURL monta a URL pública de upload TUS para video_id,
// resolvendo scheme e host a partir dos headers de proxy padrão
// (X-Forwarded-Proto, X-Forwarded-Host) com fallback para r.TLS/r.Host.
//
// Ordem de precedência para scheme:
//  1. Header X-Forwarded-Proto (ex.: "https")
//  2. r.TLS != nil → "https"
//  3. Fallback → "http"
//
// Ordem de precedência para host:
//  1. Header X-Forwarded-Host
//  2. r.Host
//
// O resultado segue o formato: <scheme>://<host>/files/<video_id>
func PublicUploadURL(r *http.Request, videoID string) string {
	scheme := resolveScheme(r)
	host := resolveHost(r)
	return fmt.Sprintf("%s://%s/files/%s", scheme, host, videoID)
}

// resolveScheme devolve o scheme (http ou https) a partir dos headers de
// proxy e do estado TLS da requisição.
func resolveScheme(r *http.Request) string {
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		return fwdProto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// resolveHost devolve o host a partir dos headers de proxy, com fallback
// para r.Host.
func resolveHost(r *http.Request) string {
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		return fwdHost
	}
	return r.Host
}
