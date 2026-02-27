package main

import (
	"testing"
)

func TestListenAddrs(t *testing.T) {
	tests := []struct {
		name     string
		bindAddr string
		port     int
		want     []string
	}{
		{
			name:     "loopback IPv4",
			bindAddr: "127.0.0.1",
			port:     3000,
			want:     []string{"127.0.0.1:3000"},
		},
		{
			name:     "loopback IPv6",
			bindAddr: "::1",
			port:     3000,
			want:     []string{"[::1]:3000"},
		},
		{
			name:     "unspecified IPv4",
			bindAddr: "0.0.0.0",
			port:     3000,
			want:     []string{"0.0.0.0:3000"},
		},
		{
			name:     "unspecified IPv6",
			bindAddr: "::",
			port:     3000,
			want:     []string{"[::]:3000"},
		},
		{
			name:     "specific non-loopback IPv4",
			bindAddr: "192.168.1.10",
			port:     3000,
			want:     []string{"192.168.1.10:3000", "127.0.0.1:3000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listenAddrs(tt.bindAddr, tt.port)
			if len(got) != len(tt.want) {
				t.Fatalf("listenAddrs(%q, %d) = %v, want %v", tt.bindAddr, tt.port, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("listenAddrs(%q, %d)[%d] = %q, want %q", tt.bindAddr, tt.port, i, got[i], tt.want[i])
				}
			}
		})
	}
}
