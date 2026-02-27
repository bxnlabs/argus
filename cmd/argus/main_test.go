package main

import (
	"testing"
)

func TestListenAddrs(t *testing.T) {
	tests := []struct {
		name string
		ips  []string
		port int
		want []string
	}{
		{
			name: "single loopback",
			ips:  []string{"127.0.0.1"},
			port: 3000,
			want: []string{"127.0.0.1:3000"},
		},
		{
			name: "IPv6 loopback",
			ips:  []string{"::1"},
			port: 3000,
			want: []string{"[::1]:3000"},
		},
		{
			name: "multiple IPs",
			ips:  []string{"192.168.1.10", "127.0.0.1"},
			port: 3000,
			want: []string{"192.168.1.10:3000", "127.0.0.1:3000"},
		},
		{
			name: "with tailscale IPs",
			ips:  []string{"127.0.0.1", "100.64.0.1", "fd7a:115c:a1e0::1"},
			port: 3000,
			want: []string{"127.0.0.1:3000", "100.64.0.1:3000", "[fd7a:115c:a1e0::1]:3000"},
		},
		{
			name: "deduplicates",
			ips:  []string{"127.0.0.1", "127.0.0.1"},
			port: 3000,
			want: []string{"127.0.0.1:3000"},
		},
		{
			name: "unspecified IPv4",
			ips:  []string{"0.0.0.0"},
			port: 3000,
			want: []string{"0.0.0.0:3000"},
		},
		{
			name: "unspecified IPv6",
			ips:  []string{"::"},
			port: 3000,
			want: []string{"[::]:3000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listenAddrs(tt.ips, tt.port)
			if len(got) != len(tt.want) {
				t.Fatalf("listenAddrs(%v, %d) = %v, want %v", tt.ips, tt.port, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("listenAddrs(%v, %d)[%d] = %q, want %q", tt.ips, tt.port, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBindIPs(t *testing.T) {
	tests := []struct {
		name     string
		bindAddr string
		extra    []string
		want     []string
	}{
		{
			name:     "loopback only",
			bindAddr: "127.0.0.1",
			want:     []string{"127.0.0.1"},
		},
		{
			name:     "specific non-loopback adds loopback",
			bindAddr: "192.168.1.10",
			want:     []string{"192.168.1.10", "127.0.0.1"},
		},
		{
			name:     "unspecified does not add loopback",
			bindAddr: "0.0.0.0",
			want:     []string{"0.0.0.0"},
		},
		{
			name:     "IPv6 loopback",
			bindAddr: "::1",
			want:     []string{"::1"},
		},
		{
			name:     "with extra tailscale IPs",
			bindAddr: "127.0.0.1",
			extra:    []string{"100.64.0.1", "fd7a:115c:a1e0::1"},
			want:     []string{"127.0.0.1", "100.64.0.1", "fd7a:115c:a1e0::1"},
		},
		{
			name:     "non-loopback with extra IPs",
			bindAddr: "192.168.1.10",
			extra:    []string{"100.64.0.1"},
			want:     []string{"192.168.1.10", "127.0.0.1", "100.64.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bindIPs(tt.bindAddr, tt.extra...)
			if len(got) != len(tt.want) {
				t.Fatalf("bindIPs(%q, %v) = %v, want %v", tt.bindAddr, tt.extra, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("bindIPs(%q, %v)[%d] = %q, want %q", tt.bindAddr, tt.extra, i, got[i], tt.want[i])
				}
			}
		})
	}
}
