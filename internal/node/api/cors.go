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
// 403 the node talking to itself. This shortcut is only reached after the Host
// gate in CORS has already validated r.Host as one of this node's own names, so
// it can no longer be abused for DNS rebinding (an evil.com Host is rejected
// before sameOrigin runs).
func sameOrigin(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// hostnameOf extracts the lowercased hostname from a request's Host header,
// dropping the port and any IPv6 brackets. "192.168.1.10:3000" → "192.168.1.10",
// "[::1]:3000" → "::1", "localhost" → "localhost".
func hostnameOf(hostHeader string) string {
	h := hostHeader
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	return strings.ToLower(h)
}

// NewHostPolicy returns a predicate reporting whether a request's Host hostname
// is a legitimate name for THIS node — the anti-DNS-rebinding gate. A browser
// always sends the Host it connected to, so restricting Host to this node's own
// names stops the rebinding attack where a page at evil.com:3000 rebinds DNS to
// 127.0.0.1: that request carries Host: evil.com:3000, which is not an allowed
// name, so it never reaches a handler (even though Origin == Host would
// otherwise look "same-origin").
//
// The allowed set is derived from what the process already knows: loopback
// always; the configured bind address when it is a concrete IP (the LAN-bind
// self-access case); and, when Tailscale is on, the node's MagicDNS FQDN plus
// any CGNAT (100.64/10) address (the node's own tailnet IP, accepted by range
// so we needn't resolve the exact one — an off-tailnet attacker can't make Host
// a CGNAT literal while rebinding). allowedHosts is the escape hatch for a
// 0.0.0.0 bind reached by a name we can't derive (an /etc/hosts alias, internal
// DNS); it is empty by default.
func NewHostPolicy(bindAddress, fqdn string, tailscaleOn bool, allowedHosts []string) func(string) bool {
	allowed := map[string]struct{}{
		"localhost": {},
		"127.0.0.1": {},
		"::1":       {},
	}
	if ip := net.ParseIP(bindAddress); ip != nil && !ip.IsUnspecified() {
		allowed[strings.ToLower(bindAddress)] = struct{}{}
	}
	if fqdn != "" {
		fqdn = strings.ToLower(fqdn)
		allowed[fqdn] = struct{}{}
		// A manual node may be added by its tailnet short name (e.g. "http://gpu"
		// — validNodeOrigin accepts these), in which case the browser sends
		// "Host: gpu" to this node. So accept our own MagicDNS short name (the
		// first label of the FQDN), not only the full FQDN. Discovery uses full
		// FQDNs and is unaffected.
		if short, _, ok := strings.Cut(fqdn, "."); ok && short != "" {
			allowed[short] = struct{}{}
		}
	}
	for _, h := range allowedHosts {
		// Normalize each entry like a request Host (drop port + IPv6 brackets,
		// lowercase) so "devbox.corp:3000" or "[fd7a::1]" actually match.
		if h = hostnameOf(strings.TrimSpace(h)); h != "" {
			allowed[h] = struct{}{}
		}
	}
	return func(host string) bool {
		if _, ok := allowed[host]; ok {
			return true
		}
		if tailscaleOn {
			if ip := net.ParseIP(host); ip != nil && cgnatBlock.Contains(ip) {
				return true
			}
		}
		return false
	}
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

// corsMaxAge is how long (seconds) a browser may cache a preflight result, so
// repeated cross-origin mutations to a remote node skip the preflight round
// trip. Conservative — well under Chrome's 7200s cap and Firefox's 86400s.
const corsMaxAge = "600"

// CORS authorizes browser requests using allowHost and allowOrigin. The Host
// gate runs first and unconditionally: a request whose Host is not one of this
// node's own names (allowHost) is rejected with 403, which closes the DNS
// rebinding path (a page at evil.com that rebinds to 127.0.0.1 sends
// Host: evil.com, an unallowed name) BEFORE any same-origin/Origin logic. After
// the Host is validated, requests without an Origin (CLI, server-to-server) and
// same-origin requests (Origin host == request Host) pass through untouched —
// the same-origin shortcut is now safe because the Host is already known good. A
// cross-origin request whose Origin is disallowed is rejected with 403 for EVERY
// method, not just preflight: CORS only stops a page from reading a response, not
// from sending a "simple" request (a bodyless POST, or a POST with a text/plain
// JSON body), and the node API has no app auth, so a denied origin must never
// reach a handler or it could trigger state-changing side effects it can't read.
func CORS(allowHost, allowOrigin func(string) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowHost(hostnameOf(r.Host)) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" || sameOrigin(r, origin) {
			next.ServeHTTP(w, r)
			return
		}
		// Vary on Origin regardless of the decision so caches never serve a
		// CORS-headerless response to a later allowed origin.
		w.Header().Set("Vary", "Origin")
		if !allowOrigin(origin) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", corsMaxAge)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
