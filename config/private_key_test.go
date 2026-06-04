package config

import "testing"

func TestNormalizePrivateKey(t *testing.T) {
	tests := []struct {
		name       string
		privateKey string
		want       string
	}{
		{
			name:       "without prefix",
			privateKey: "abc123",
			want:       "abc123",
		},
		{
			name:       "with lowercase prefix",
			privateKey: "0xabc123",
			want:       "abc123",
		},
		{
			name:       "with uppercase prefix",
			privateKey: "0Xabc123",
			want:       "abc123",
		},
		{
			name:       "with surrounding whitespace",
			privateKey: "\n 0xabc123 \t",
			want:       "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePrivateKey(tt.privateKey)
			if got != tt.want {
				t.Fatalf("NormalizePrivateKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
