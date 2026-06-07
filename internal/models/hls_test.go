package models

import "testing"

// TestSegmentNameRe_TableDriven verifica que a regex centralizada de nome
// de segmento HLS casa exatamente os padrões esperados e rejeita variações
// inválidas — documenta o contrato compartilhado entre o worker de
// transcodificação e o serving estático.
func TestSegmentNameRe_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK bool
	}{
		{"segmento simples", "0.ts", true},
		{"segmento com vários dígitos", "123.ts", true},
		{"segmento com zero à esquerda", "01.ts", true},
		{"segmento máximo", "999999.ts", true},
		{"letras no lugar de dígitos", "abc.ts", false},
		{"extensão errada m4s", "1.m4s", false},
		{"maiúsculas na extensão", "1.TS", false},
		{"path traversal", "../1.ts", false},
		{"string vazia", "", false},
		{"só dígitos sem extensão", "123", false},
		{"extensão sem dígitos", ".ts", false},
		{"playlist (não é segmento)", "playlist.m3u8", false},
		{"dígitos com .ts no meio", "1.ts.something", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SegmentNameRe.MatchString(tc.input)
			if got != tc.wantOK {
				t.Errorf("SegmentNameRe.MatchString(%q): esperado %v, obtido %v",
					tc.input, tc.wantOK, got)
			}
		})
	}
}
