package conf

import (
	"net"
	"testing"
)

func TestParseMAC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    net.HardwareAddr
		wantErr bool
	}{
		{
			name:  "canonical colon format",
			input: "e0:63:da:c7:39:43",
			want:  net.HardwareAddr{0xe0, 0x63, 0xda, 0xc7, 0x39, 0x43},
		},
		{
			name:  "single octet shorthand from macOS arp output",
			input: "1c:b:8b:3e:9c:60",
			want:  net.HardwareAddr{0x1c, 0x0b, 0x8b, 0x3e, 0x9c, 0x60},
		},
		{
			name:  "single octet shorthand with hyphen separator",
			input: "1C-B-8B-3E-9C-60",
			want:  net.HardwareAddr{0x1c, 0x0b, 0x8b, 0x3e, 0x9c, 0x60},
		},
		{
			name:  "dot separated format remains supported",
			input: "1c0b.8b3e.9c60",
			want:  net.HardwareAddr{0x1c, 0x0b, 0x8b, 0x3e, 0x9c, 0x60},
		},
		{
			name:    "invalid hex",
			input:   "1c:zz:8b:3e:9c:60",
			wantErr: true,
		},
		{
			name:    "invalid part count",
			input:   "1c:b:8b:3e:9c",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseMAC(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none (parsed %s)", got.String())
				}
				return
			}

			if err != nil {
				t.Fatalf("parseMAC(%q) returned error: %v", tc.input, err)
			}
			if got.String() != tc.want.String() {
				t.Fatalf("parseMAC(%q) = %s, want %s", tc.input, got.String(), tc.want.String())
			}
		})
	}
}
