package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// cgnatBlock is Tailscale's CGNAT range (100.64.0.0/10). Parsed once so the
// per-request origin check is a cheap Contains.
var cgnatBlock = func() *net.IPNet {
	_, n, _ := net.ParseCIDR("100.64.0.0/10")
	return n
}()

// originHost parses an Origin header value and returns its lowercased hostname
// (no port, IPv6 brackets stripped). ok is false when the value isn't a usable
// origin.
func originHost(origin string) (host string, ok bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return "", false
	}
	return strings.ToLower(u.Hostname()), true
}

// sameOrigin reports whether origin's host:port matches the request's Host —
// i.e. the request is same-origin (the SPA calling its own node). Same-origin
// requests are not cross-origin at all and must pass regardless of the
// allow-list: a non-loopback bind (a LAN IP or 0.0.0.0) serves the SPA from an
// Origin that is neither loopback nor tailnet, so the allow-list alone would
// 403 the node talking to itself.
func sameOrigin(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// isLoopbackOrigin reports whether origin points at this machine (localhost /
// 127.0.0.1 / ::1, any port). This is the local SPA and the Vite dev proxy.
// The check is on the Origin header's host, not the request's socket, so a DNS
// rebind of evil.com → 127.0.0.1 still fails (its Origin stays http://evil.com).
func isLoopbackOrigin(origin string) bool {
	host, ok := originHost(origin)
	if !ok {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isTailnetOrigin reports whether origin is a tailnet peer: a Tailscale CGNAT
// IP (100.64.0.0/10) or a MagicDNS name under tailnetSuffix (e.g.
// ".tail1234.ts.net"). An empty suffix means Tailscale is off, so nothing is a
// tailnet origin (not even a CGNAT IP — there's no tailnet to belong to).
func isTailnetOrigin(origin, tailnetSuffix string) bool {
	if tailnetSuffix == "" {
		return false
	}
	host, ok := originHost(origin)
	if !ok {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return cgnatBlock.Contains(ip)
	}
	// tailnetSuffix carries its leading dot, so "evilts.net" can't match
	// ".ts.net" and only a true subdomain of the full tailnet name passes.
	return strings.HasSuffix(host, strings.ToLower(tailnetSuffix))
}

// NewCORSPolicy returns an allow-predicate for cross-origin requests. It permits
// loopback origins always and tailnet origins when tailnetSuffix is set. Every
// other origin (e.g. a malicious public page) is rejected — the node API has no
// app auth, so reflecting arbitrary origins would let any page the user visits
// read/write files and create sessions on a reachable node.
func NewCORSPolicy(tailnetSuffix string) func(string) bool {
	return func(origin string) bool {
		return isLoopbackOrigin(origin) || isTailnetOrigin(origin, tailnetSuffix)
	}
}

// CORS authorizes cross-origin browser requests using allow. Requests without
// an Origin (CLI, server-to-server, same-origin GET) and same-origin requests
// (Origin host == request Host) pass through untouched. A request whose Origin
// is cross-origin and disallowed is rejected outright with 403 for EVERY
// method — not just preflight. CORS only stops a page from reading a response;
// it does not stop the browser from sending a "simple" request (a bodyless
// POST, or a POST with a text/plain JSON body), and the node API has no app
// auth, so a denied origin must never reach a handler or it could trigger
// state-changing side effects (create/acknowledge sessions) it just can't read.
func CORS(allow func(string) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || sameOrigin(r, origin) {
			next.ServeHTTP(w, r)
			return
		}
		// Vary on Origin regardless of the decision so caches never serve a
		// CORS-headerless response to a later allowed origin.
		w.Header().Set("Vary", "Origin")
		if !allow(origin) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
