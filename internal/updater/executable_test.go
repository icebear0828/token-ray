package updater

import "testing"

func TestLooksLikeExecutable(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"elf", []byte{0x7f, 'E', 'L', 'F', 1, 2, 3, 4}, true},
		{"pe", []byte{'M', 'Z', 0, 0}, true},
		{"macho64", []byte{0xcf, 0xfa, 0xed, 0xfe}, true},
		{"checksum text", []byte("abc123  ai-flight-dashboard-linux-amd64\n"), false},
		{"too short", []byte{0x7f}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeExecutable(tt.data); got != tt.want {
				t.Errorf("looksLikeExecutable(%q) = %v; want %v", tt.data, got, tt.want)
			}
		})
	}
}
