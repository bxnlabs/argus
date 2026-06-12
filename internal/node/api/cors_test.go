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

func TestHostnameOf(t *testing.T) {
	tests := []struct{ host, want string }{
		{"localhost:3000", "localhost"},
		{"localhost", "localhost"},
		{"192.168.1.10:3000", "192.168.1.10"},
		{"[::1]:3000", "::1"},
		{"[::1]", "::1"},
		{"GPU-Box.tail1234.ts.net", "gpu-box.tail1234.ts.net"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := hostnameOf(tt.host); got != tt.want {
			t.Errorf("hostnameOf(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestNewHostPolicy(t *testing.T) {
	t.Run("loopback only by default", func(t *testing.T) {
		allow := NewHostPolicy("", "", false, nil)
		for _, h := range []string{"localhost", "127.0.0.1", "::1"} {
			if !allow(h) {
				t.Errorf("loopback host %q should be allowed", h)
			}
		}
		for _, h := range []string{"evil.com", "192.168.1.10", "100.64.0.1"} {
			if allow(h) {
				t.Errorf("host %q should be rejected by the loopback-only policy", h)
			}
		}
	})

	t.Run("concrete bind address allowed, unspecified skipped", func(t *testing.T) {
		allow := NewHostPolicy("192.168.1.10", "", false, nil)
		if !allow("192.168.1.10") {
			t.Error("the concrete bind address should be an allowed Host")
		}
		// A 0.0.0.0 bind contributes no Host (it's not a name a browser sends).
		allowWild := NewHostPolicy("0.0.0.0", "", false, nil)
		if allowWild("0.0.0.0") {
			t.Error("unspecified bind address must not be added as an allowed Host")
		}
	})

	t.Run("tailnet FQDN, short name, and CGNAT range allowed when on", func(t *testing.T) {
		allow := NewHostPolicy("127.0.0.1", "gpu-box.tail1234.ts.net", true, nil)
		if !allow("gpu-box.tail1234.ts.net") {
			t.Error("the node's own FQDN should be an allowed Host")
		}
		// Manual nodes can be added by tailnet short name, so the short name
		// (first FQDN label) must be an allowed Host too.
		if !allow("gpu-box") {
			t.Error("the node's MagicDNS short name should be an allowed Host")
		}
		if !allow("100.64.0.7") {
			t.Error("a CGNAT Host should be allowed when Tailscale is on")
		}
		// Off: no CGNAT range, no FQDN/short name.
		off := NewHostPolicy("127.0.0.1", "", false, nil)
		if off("100.64.0.7") {
			t.Error("CGNAT Host must be rejected when Tailscale is off")
		}
	})

	t.Run("allowed_hosts escape hatch (normalized like request Hosts)", func(t *testing.T) {
		allow := NewHostPolicy("0.0.0.0", "", false, []string{
			"devbox.corp:3000", // port stripped to match hostnameOf(r.Host)
			"  ",
			"Alias.Local",
			"[fd7a::1]", // IPv6 brackets stripped
		})
		if !allow("devbox.corp") {
			t.Error("an allowed_hosts entry with a port should match the bare hostname")
		}
		if !allow("alias.local") {
			t.Error("allowed_hosts entries should be matched case-insensitively")
		}
		if !allow("fd7a::1") {
			t.Error("a bracketed IPv6 allowed_hosts entry should match the unbracketed Host")
		}
		if allow("other.corp") {
			t.Error("a host not in allowed_hosts must be rejected")
		}
	})
}

func okHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called != nil {
			*called = true
		}
		w.WriteHeader(http.StatusOK)
	})
}

// loopbackHosts is the default Host policy used by Origin-focused tests: it
// admits localhost/127.0.0.1/::1, so requests must arrive with a loopback Host.
func loopbackHosts() func(string) bool { return NewHostPolicy("127.0.0.1", "", false, nil) }

func TestCORS_AllowedOrigin(t *testing.T) {
	h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(nil))

	// Real dev shape: the SPA on the Vite port calls the node on its own port —
	// same loopback host, different port, so it's a genuine cross-origin request.
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Host = "localhost:3000"
	req.Header.Set("Origin", "http://localhost:5273")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5273" {
		t.Errorf("Allow-Origin = %q, want reflected loopback origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != corsMaxAge {
		t.Errorf("Max-Age = %q, want %q", got, corsMaxAge)
	}
}

func TestCORS_DeniedOrigin_NonPreflight(t *testing.T) {
	// A denied origin must be rejected for every method, not just preflight:
	// a bodyless/text-plain POST is a CORS "simple" request the browser sends
	// without a preflight, so reaching the handler would mutate state.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		called := false
		h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(&called))

		req := httptest.NewRequest(method, "/sessions", nil)
		req.Host = "localhost:3000"
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
	h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodOptions, "/sessions", nil)
	req.Host = "localhost:3000"
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
	h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodOptions, "/sessions", nil)
	req.Host = "127.0.0.1:3000"
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
	if got := rec.Header().Get("Access-Control-Max-Age"); got != corsMaxAge {
		t.Errorf("Max-Age = %q, want %q", got, corsMaxAge)
	}
}

func TestCORS_SameOrigin_LANBind_AllowedHost(t *testing.T) {
	// A LAN/0.0.0.0 bind serves the SPA from a non-loopback, non-tailnet Origin.
	// Those same-origin requests (Origin host == Host) must pass — but only once
	// the Host is a known node name (here, the configured bind address). The
	// allow-list would otherwise reject the origin as cross-origin.
	allowHost := NewHostPolicy("192.168.1.10", "", false, nil)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		called := false
		h := CORS(allowHost, NewCORSPolicy(""), okHandler(&called))

		req := httptest.NewRequest(method, "/sessions", nil)
		req.Host = "192.168.1.10:3000"
		req.Header.Set("Origin", "http://192.168.1.10:3000")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s same-origin status = %d, want 200", method, rec.Code)
		}
		if !called {
			t.Errorf("%s same-origin request must reach the handler", method)
		}
	}
}

func TestCORS_Rebinding_Denied(t *testing.T) {
	// The DNS-rebinding regression: a page at evil.com:3000 rebinds to 127.0.0.1.
	// Its requests carry Origin == Host == evil.com:3000, which would pass the
	// same-origin shortcut — but the Host gate rejects evil.com first, so the
	// request never reaches a handler. This is the test that would have caught
	// the original vulnerability.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		called := false
		h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(&called))

		req := httptest.NewRequest(method, "/sessions", nil)
		req.Host = "evil.com:3000"
		req.Header.Set("Origin", "http://evil.com:3000")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s rebinding status = %d, want 403", method, rec.Code)
		}
		if called {
			t.Errorf("%s rebinding request must not reach the handler", method)
		}
	}
}

func TestCORS_Rebinding_WebSocketRoute(t *testing.T) {
	// The Host gate wraps the whole mux, including the terminal WebSocket routes.
	// A rebinding handshake against /ws/terminal must be rejected too.
	called := false
	h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/ws/terminal", nil)
	req.Host = "evil.com:3000"
	req.Header.Set("Origin", "http://evil.com:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("rebinding WS status = %d, want 403", rec.Code)
	}
	if called {
		t.Error("rebinding WS handshake must not reach the handler")
	}
}

func TestCORS_LANBind_DisallowedHost(t *testing.T) {
	// Same LAN bind, but the request's Host is not the configured bind address
	// (and not loopback/tailnet). Even though Origin == Host (which the deleted
	// shortcut would have honored), the Host gate rejects it — proving the
	// same-origin path can't be used to bypass Host validation.
	called := false
	h := CORS(NewHostPolicy("192.168.1.10", "", false, nil), NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	req.Host = "192.168.1.99:3000"
	req.Header.Set("Origin", "http://192.168.1.99:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("disallowed-host status = %d, want 403", rec.Code)
	}
	if called {
		t.Error("a request with an unallowed Host must not reach the handler")
	}
}

func TestCORS_CrossOrigin_NonLoopbackBind_StillDenied(t *testing.T) {
	// Allowed Host (the bind address), but the Origin's host differs from Host:
	// a real cross-origin request that must still be rejected by the allow-list.
	called := false
	h := CORS(NewHostPolicy("192.168.1.10", "", false, nil), NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	req.Host = "192.168.1.10:3000"
	req.Header.Set("Origin", "http://192.168.1.99:3000")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin status = %d, want 403", rec.Code)
	}
	if called {
		t.Error("cross-origin request from a denied origin must not reach the handler")
	}
}

func TestCORS_NoOrigin(t *testing.T) {
	called := false
	h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Host = "localhost:3000"
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

func TestCORS_NoOrigin_BadHost(t *testing.T) {
	// The Host gate runs before the no-Origin passthrough, so a non-browser
	// (or Origin-stripped) rebinding attempt can't slip through on a bad Host.
	called := false
	h := CORS(loopbackHosts(), NewCORSPolicy(""), okHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Host = "evil.com:3000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("no-Origin bad-Host status = %d, want 403", rec.Code)
	}
	if called {
		t.Error("a request with an unallowed Host must not reach the handler, even without an Origin")
	}
}

func TestCORS_TailnetOrigin(t *testing.T) {
	allowHost := NewHostPolicy("127.0.0.1", "gpu-box.tail1234.ts.net", true, nil)
	h := CORS(allowHost, NewCORSPolicy(".tail1234.ts.net"), okHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Host = "gpu-box.tail1234.ts.net"
	req.Header.Set("Origin", "http://peer-box.tail1234.ts.net")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://peer-box.tail1234.ts.net" {
		t.Errorf("Allow-Origin = %q, want reflected tailnet origin", got)
	}
}
