package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:5273", true},
		{"http://localhost", true},
		{"http://127.0.0.1:3000", true},
		{"http://[::1]:3000", true},
		{"https://evil.com", false},
		{"http://100.64.1.2", false}, // tailnet, not loopback
		{"", false},
		{"::not a url::", false},
	}
	for _, tt := range tests {
		if got := isLoopbackOrigin(tt.origin); got != tt.want {
			t.Errorf("isLoopbackOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
		}
	}
}

func TestIsTailnetOrigin(t *testing.T) {
	const suffix = ".tail1234.ts.net"
	tests := []struct {
		origin string
		want   bool
	}{
		{"http://peer-node.tail1234.ts.net", true},
		{"http://peer-node.TAIL1234.TS.NET", true},   // case-insensitive
		{"http://100.64.1.5", true},                  // CGNAT
		{"http://100.127.255.255:3000", true},        // CGNAT upper bound
		{"http://100.128.0.1", false},                // just outside CGNAT
		{"http://10.0.0.1", false},                   // private, not tailnet
		{"http://evilts.net", false},                 // no dot-anchored match
		{"http://x.tail1234.ts.net.evil.com", false}, // suffix not at end
		{"http://localhost", false},                  // loopback's job
		{"", false},
	}
	for _, tt := range tests {
		if got := isTailnetOrigin(tt.origin, suffix); got != tt.want {
			t.Errorf("isTailnetOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
		}
	}
	// Empty suffix means Tailscale is off: nothing is a tailnet origin, not
	// even a CGNAT IP (there's no tailnet to belong to).
	if isTailnetOrigin("http://peer.tail1234.ts.net", "") {
		t.Error("empty suffix should reject MagicDNS names")
	}
	if isTailnetOrigin("http://100.64.0.1", "") {
		t.Error("empty suffix should reject CGNAT IPs (Tailscale off → loopback-only)")
	}
}

func okHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called != nil {
			*called = true
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestCORS_AllowedOrigin(t *testing.T) {
	h := CORS(NewCORSPolicy(""), okHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("Origin", "http://localhost:5273")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5273" {
		t.Errorf("Allow-Origin = %q, want reflected loopback origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

func TestCORS_DeniedOrigin_NonPreflight(t *testing.T) {
	// A denied origin must be rejected for every method, not just preflight:
	// a bodyless/text-plain POST is a CORS "simple" request the browser sends
	// without a preflight, so reaching the handler would mutate state.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		called := false
		h := CORS(NewCORSPolicy(""), okHandler(&called))

		req := httptest.NewRequest(method, "/sessions", nil)
		req.Header.Set("Origin", "https://evil.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s denied status = %d, want 403", method, rec.Code)
		}
		if called {
			t.Errorf("%s from a denied origin must not reach the handler", method)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s Allow-Origin = %q, want empty for denied origin", method, got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Errorf("%s Vary = %q, want Origin", method, got)
		}
	}
}

func TestCORS_DeniedOrigin_Preflight(t *testing.T) {
	called := false
	h := CORS(NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodOptions, "/sessions", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("denied preflight status = %d, want 403", rec.Code)
	}
	if called {
		t.Error("denied preflight must not reach the handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for denied preflight", got)
	}
}

func TestCORS_AllowedOrigin_Preflight(t *testing.T) {
	called := false
	h := CORS(NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodOptions, "/sessions", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5273")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("allowed preflight status = %d, want 204", rec.Code)
	}
	if called {
		t.Error("preflight must not reach the wrapped handler")
	}
}

func TestCORS_NoOrigin(t *testing.T) {
	called := false
	h := CORS(NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("no-Origin request (CLI/server-to-server) must reach the handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty when no Origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q, want empty when no Origin", got)
	}
}

func TestCORS_TailnetOrigin(t *testing.T) {
	h := CORS(NewCORSPolicy(".tail1234.ts.net"), okHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("Origin", "http://gpu-box.tail1234.ts.net")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://gpu-box.tail1234.ts.net" {
		t.Errorf("Allow-Origin = %q, want reflected tailnet origin", got)
	}
}
