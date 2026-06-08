package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/api"
	"github.com/bxnlabs/argus/internal/node/db"
)

type memDB struct{ nodes []ManualNode }

func (m *memDB) ListManualNodes(context.Context) ([]ManualNode, error) { return m.nodes, nil }
func (m *memDB) AddManualNode(_ context.Context, id, name, url string) error {
	for _, n := range m.nodes {
		if n.URL == url {
			return fmt.Errorf("%w: %s", db.ErrDuplicateURL, url)
		}
	}
	m.nodes = append(m.nodes, ManualNode{ID: id, Name: name, URL: url})
	return nil
}
func (m *memDB) RenameManualNode(_ context.Context, id, name string) error {
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			m.nodes[i].Name = name
			return nil
		}
	}
	return db.ErrNotFound
}
func (m *memDB) DeleteManualNode(_ context.Context, id string) error {
	out := m.nodes[:0]
	found := false
	for _, n := range m.nodes {
		if n.ID != id {
			out = append(out, n)
		} else {
			found = true
		}
	}
	m.nodes = out
	if !found {
		return db.ErrNotFound
	}
	return nil
}

func newCounter() func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	}
}

func newTestHandlers() (*Handlers, *memDB) {
	mdb := &memDB{}
	svc := New(mdb, Node{ID: "self", Name: "this", Source: SourceLocal, Self: true}, nil)
	return NewHandlers(svc, mdb, newCounter(), nil), mdb
}

// TestRegisterAppliesCORS asserts the registry routes are wrapped in the
// supplied origin guard: a disallowed cross-origin request is rejected with 403
// before reaching the handler, so it cannot mutate the registry (CSRF defense).
func TestRegisterAppliesCORS(t *testing.T) {
	mdb := &memDB{}
	svc := New(mdb, Node{ID: "self", Name: "this", Source: SourceLocal, Self: true}, nil)
	// Empty tailnet suffix → loopback-only policy, so http://evil.com is denied.
	cors := func(next http.Handler) http.Handler { return api.CORS(api.NewCORSPolicy(""), next) }
	h := NewHandlers(svc, mdb, newCounter(), cors)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu:80"}`))
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(mdb.nodes) != 0 {
		t.Errorf("registry mutated despite forbidden origin: %+v", mdb.nodes)
	}
}

// TestRegisterHandlesPreflight asserts a CORS preflight reaches the guard
// (rather than falling through to a SPA catch-all): a disallowed origin gets
// 403, an allowed loopback origin gets 204 with the reflected ACAO header.
func TestRegisterHandlesPreflight(t *testing.T) {
	mdb := &memDB{}
	svc := New(mdb, Node{ID: "self", Name: "this", Source: SourceLocal, Self: true}, nil)
	cors := func(next http.Handler) http.Handler { return api.CORS(api.NewCORSPolicy(""), next) }
	h := NewHandlers(svc, mdb, newCounter(), cors)
	mux := http.NewServeMux()
	h.Register(mux)

	preflight := func(path, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("OPTIONS", path, nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := preflight("/api/nodes", "http://evil.com"); rec.Code != http.StatusForbidden {
		t.Errorf("disallowed preflight on /api/nodes: status = %d, want 403", rec.Code)
	}
	if rec := preflight("/api/nodes/id-1", "http://evil.com"); rec.Code != http.StatusForbidden {
		t.Errorf("disallowed preflight on /api/nodes/{id}: status = %d, want 403", rec.Code)
	}
	if rec := preflight("/api/nodes", "http://localhost:5273"); rec.Code != http.StatusNoContent {
		t.Errorf("allowed preflight: status = %d, want 204", rec.Code)
	} else if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5273" {
		t.Errorf("allowed preflight ACAO = %q, want %q", got, "http://localhost:5273")
	}
}

func TestList_ReturnsSelf(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nodes", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct{ Nodes []Node }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Nodes) != 1 || !body.Nodes[0].Self {
		t.Errorf("nodes = %+v", body.Nodes)
	}
}

func TestAddThenList(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu:80"}`)))
	if rec.Code != 201 {
		t.Fatalf("add status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nodes", nil))
	var body struct{ Nodes []Node }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Nodes) != 2 {
		t.Errorf("len = %d, want 2 (self+gpu)", len(body.Nodes))
	}
}

func TestAddRejectsBadURL(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"x","url":"not-a-url"}`)))
	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAddRejectsNonOriginURL(t *testing.T) {
	// A node URL is used as an origin/base; path, query, fragment, and userinfo
	// would corrupt later requests (e.g. http://gpu/api/api/node/summary).
	cases := []string{
		`{"name":"x","url":"http://gpu/api"}`,
		`{"name":"x","url":"http://gpu/?q=1"}`,
		`{"name":"x","url":"http://gpu/#frag"}`,
		`{"name":"x","url":"http://user:pass@gpu"}`,
	}
	for _, body := range cases {
		h, _ := newTestHandlers()
		mux := http.NewServeMux()
		h.Register(mux)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes", strings.NewReader(body)))
		if rec.Code != 400 {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestAddAcceptsBareOriginWithTrailingSlash(t *testing.T) {
	// A trailing slash is the only path component allowed; it is trimmed.
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu/"}`)))
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAddDuplicateReturns409(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	// First add succeeds.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu:80"}`)))
	if rec.Code != 201 {
		t.Fatalf("first add status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Second add with the same URL returns 409.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu-copy","url":"http://gpu:80"}`)))
	if rec.Code != 409 {
		t.Errorf("duplicate add status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAddNormalizesAndRejectsVariantDuplicate(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	// First add: uppercase host + explicit default port → normalized to http://gpu-box
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://GPU-BOX:80"}`)))
	if rec.Code != 201 {
		t.Fatalf("first add status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Second add: lowercase host, no port — also normalizes to http://gpu-box → 409.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu-copy","url":"http://gpu-box"}`)))
	if rec.Code != 409 {
		t.Errorf("variant duplicate status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestRenameMissingReturns404(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/nonexistent",
		strings.NewReader(`{"name":"new-name"}`)))
	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteMissingReturns404(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/nodes/nonexistent", nil))
	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestRenameAndDeleteSucceed(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	// Add a node — counter returns "id-1" on first call.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu:80"}`)))
	if rec.Code != 201 {
		t.Fatalf("add status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Rename it.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/id-1",
		strings.NewReader(`{"name":"gpu-renamed"}`)))
	if rec.Code != 204 {
		t.Fatalf("rename status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Delete it.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/nodes/id-1", nil))
	if rec.Code != 204 {
		t.Fatalf("delete status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Final GET: only self remains.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nodes", nil))
	var body struct{ Nodes []Node }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Nodes) != 1 || !body.Nodes[0].Self {
		t.Errorf("after delete: nodes = %+v, want only self", body.Nodes)
	}
}
