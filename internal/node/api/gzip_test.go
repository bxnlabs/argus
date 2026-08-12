package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func echoBody(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}
}

func TestGzipped(t *testing.T) {
	// Repetitive enough to compress, like the diffs and source files this
	// wrapper is applied to.
	body := strings.Repeat(`{"line":"the quick brown fox"},`, 200)

	t.Run("compresses when the client accepts gzip", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		w := httptest.NewRecorder()
		gzipped(echoBody(body))(w, req)

		if got := w.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
			t.Errorf("Vary = %q, want Accept-Encoding", got)
		}
		if w.Body.Len() >= len(body) {
			t.Errorf("compressed body is %d bytes, want fewer than %d", w.Body.Len(), len(body))
		}

		gr, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Fatalf("response is not valid gzip: %v", err)
		}
		got, err := io.ReadAll(gr)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Error("decompressed body does not match what the handler wrote")
		}
	})

	t.Run("passes through when the client does not accept gzip", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/x", nil)
		w := httptest.NewRecorder()
		gzipped(echoBody(body))(w, req)

		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none", got)
		}
		if w.Body.String() != body {
			t.Error("body was altered for a client that asked for no encoding")
		}
		// The identity response varies on Accept-Encoding too: a cache told
		// otherwise may hand this stored body to a client that asked for gzip.
		if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
			t.Errorf("Vary = %q, want Accept-Encoding", got)
		}
	})

	// "gzip;q=0" is a client refusing gzip, and a substring check reads it as
	// acceptance — which would send bytes the client told us it cannot decode.
	t.Run("honours a zero quality value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Accept-Encoding", "gzip;q=0, identity")
		w := httptest.NewRecorder()
		gzipped(echoBody(body))(w, req)

		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none", got)
		}
		if w.Body.String() != body {
			t.Error("body was compressed for a client that refused gzip")
		}
	})

	// The spec writes quality as a fixed-point number and the coding token as
	// case-insensitive, so a refusal has more than one legal spelling. Matching
	// only the literal "q=0" against a literal lowercase "gzip" answers most of
	// them with a compressed body the client cannot decode.
	t.Run("honours every legal spelling of a refusal", func(t *testing.T) {
		for _, header := range []string{
			"gzip;q=0",
			"gzip;q=0.0",
			"gzip;q=0.000",
			"gzip; q=0.0",
			"GZip;q=0",
			"gzip;Q=0.0",
		} {
			req := httptest.NewRequest("GET", "/x", nil)
			req.Header.Set("Accept-Encoding", header+", identity")
			w := httptest.NewRecorder()
			gzipped(echoBody(body))(w, req)

			if got := w.Header().Get("Content-Encoding"); got != "" {
				t.Errorf("Accept-Encoding %q: Content-Encoding = %q, want none", header, got)
			}
			if w.Body.String() != body {
				t.Errorf("Accept-Encoding %q: body was compressed for a client that refused gzip", header)
			}
		}
	})

	// A non-zero quality is acceptance, and so is a malformed one — the client
	// named gzip either way, and only an explicit zero withdraws it.
	t.Run("compresses for a non-zero or unparseable quality", func(t *testing.T) {
		for _, header := range []string{
			"gzip;q=1.0",
			"gzip;q=0.5",
			"GZIP",
			"gzip;q=nonsense",
		} {
			req := httptest.NewRequest("GET", "/x", nil)
			req.Header.Set("Accept-Encoding", header)
			w := httptest.NewRecorder()
			gzipped(echoBody(body))(w, req)

			if got := w.Header().Get("Content-Encoding"); got != "gzip" {
				t.Errorf("Accept-Encoding %q: Content-Encoding = %q, want gzip", header, got)
			}
		}
	})

	// A repeated list-valued field is one list split across lines, so the token
	// can arrive on any of them. Header.Get reads only the first and would miss
	// this — the property is worth pinning, since nothing else fails if the
	// parser is simplified back to it.
	t.Run("reads a token from a repeated header line", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Add("Accept-Encoding", "br")
		req.Header.Add("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		gzipped(echoBody(body))(w, req)

		if got := w.Header().Get("Content-Encoding"); got != "gzip" {
			t.Errorf("Content-Encoding = %q, want gzip", got)
		}
	})

	t.Run("preserves the handler's status code", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		gzipped(func(w http.ResponseWriter, r *http.Request) {
			respondError(w, http.StatusNotFound, "file not found")
		})(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

// The reason this is a per-route wrapper rather than middleware over the whole
// mux: the same mux serves the terminal WebSocket, and a ResponseWriter that
// does not forward Hijack fails the upgrade. Nothing stops a future edit from
// wrapping the mux, so pin the property that would break.
func TestWebSocketRoutesAreNotWrapped(t *testing.T) {
	router := NewRouter(Deps{})

	req := httptest.NewRequest("GET", "/ws/terminal", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q on a WebSocket route, want none", got)
	}
}
