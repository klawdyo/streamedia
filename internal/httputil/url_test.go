package httputil

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPublicUploadURL_TableDriven verifica que a função centralizada de
// construção de URL pública resolve corretamente scheme e host a partir
// dos headers de proxy e do estado TLS — documenta o contrato antes
// duplicado entre /upload/init e /admin/projects/{slug}/upload-tokens.
func TestPublicUploadURL_TableDriven(t *testing.T) {
	const videoID = "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name       string
		host       string
		tls        bool
		fwdProto   string
		fwdHost    string
		wantURL    string
	}{
		{
			name:    "sem proxy, sem TLS",
			host:    "example.com",
			wantURL: "http://example.com/files/" + videoID,
		},
		{
			name:    "sem proxy, com TLS",
			host:    "example.com",
			tls:     true,
			wantURL: "https://example.com/files/" + videoID,
		},
		{
			name:     "com X-Forwarded-Proto https",
			host:     "example.com",
			fwdProto: "https",
			wantURL:  "https://example.com/files/" + videoID,
		},
		{
			name:    "com X-Forwarded-Host",
			host:    "example.com",
			fwdHost: "cdn.example.com",
			wantURL: "http://cdn.example.com/files/" + videoID,
		},
		{
			name:     "com ambos os headers",
			host:     "example.com",
			fwdProto: "https",
			fwdHost:  "cdn.example.com",
			wantURL:  "https://cdn.example.com/files/" + videoID,
		},
		{
			name:     "X-Forwarded-Proto prevalece sobre TLS",
			host:     "example.com",
			tls:      true,
			fwdProto: "http",
			wantURL:  "http://example.com/files/" + videoID,
		},
		{
			name:    "host com porta",
			host:    "localhost:3000",
			wantURL: "http://localhost:3000/files/" + videoID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tc.host

			if tc.fwdProto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.fwdProto)
			}
			if tc.fwdHost != "" {
				req.Header.Set("X-Forwarded-Host", tc.fwdHost)
			}
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}

			got := PublicUploadURL(req, videoID)
			if got != tc.wantURL {
				t.Errorf("PublicUploadURL: esperado %q, obtido %q", tc.wantURL, got)
			}
		})
	}
}

// TestResolveScheme testes isolados para a lógica de resolução de scheme.
func TestResolveScheme(t *testing.T) {
	tests := []struct {
		name     string
		fwdProto string
		tls      bool
		want     string
	}{
		{"sem proxy, sem TLS", "", false, "http"},
		{"sem proxy, com TLS", "", true, "https"},
		{"X-Forwarded-Proto http", "http", false, "http"},
		{"X-Forwarded-Proto https", "https", false, "https"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.fwdProto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.fwdProto)
			}
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			got := resolveScheme(req)
			if got != tc.want {
				t.Errorf("resolveScheme: esperado %q, obtido %q", tc.want, got)
			}
		})
	}
}
