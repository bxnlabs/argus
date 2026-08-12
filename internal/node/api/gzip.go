package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// gzipped compresses a handler's response when the client accepts it.
//
// Applied per route rather than to the whole mux, for two reasons. The mux
// also serves the terminal WebSocket, and wrapping that ResponseWriter hides
// the http.Hijacker the upgrade needs — the handshake fails outright. And most
// routes here answer with a few hundred bytes of JSON, where gzip's framing is
// a large fraction of the payload; only the routes that carry bulk are worth
// it. That keeps this free of the buffer-until-a-threshold machinery a global
// middleware would need.
func gzipped(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Announced on both branches. The identity response is just as
		// dependent on Accept-Encoding as the compressed one — a cache told
		// only about the latter is free to serve a stored identity body to a
		// client that asked for gzip, or the reverse.
		w.Header().Add("Vary", "Accept-Encoding")

		if !acceptsGzip(r) {
			h(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")

		gz := gzip.NewWriter(w)
		// Runs after h returns and before this handler does, so the trailer is
		// written while the response is still being assembled.
		defer gz.Close()

		h(&gzipResponseWriter{ResponseWriter: w, body: gz}, r)
	}
}

// acceptsGzip reports whether the client asked for gzip and did not then
// reject it with a zero quality value ("gzip;q=0").
//
// Both comparisons are looser than they look like they need to be, and both
// have to be. RFC 9110 makes the coding token case-insensitive, and it spells
// the quality as a fixed-point number — so "GZip", "q=0.0" and "q=0.000" are
// all things a client may legitimately send, and matching only the literal
// lowercase token or the literal text "q=0" answers a refusal with a
// compressed body the client already said it could not decode.
func acceptsGzip(r *http.Request) bool {
	// Values, not Get: a repeated list-valued field is one list split across
	// lines, and Get would read only the first of them.
	header := strings.Join(r.Header.Values("Accept-Encoding"), ",")

	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		for _, param := range fields[1:] {
			name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") {
				continue
			}
			// An unparseable quality is not a refusal: the parameter is
			// malformed, and the client did still name gzip.
			if q, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && q <= 0 {
				return false
			}
		}
		return true
	}
	return false
}

// gzipResponseWriter sends the body through gzip while leaving header and
// status handling to the underlying writer.
type gzipResponseWriter struct {
	http.ResponseWriter
	body io.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.body.Write(b)
}
