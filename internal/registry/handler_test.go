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
func (m *memDB) UpdateManualNode(_ context.Context, id, name, url string) error {
	for _, n := range m.nodes {
		if n.URL == url && n.ID != id {
			return fmt.Errorf("%w: %s", db.ErrDuplicateURL, url)
		}
	}
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			m.nodes[i].Name = name
			m.nodes[i].URL = url
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
	svc := New(mdb, Node{ID: "self", Name: "this", Source: SourceLocal, Self: true}, "", nil)
	return NewHandlers(svc, newCounter(), nil), mdb
}

// TestRegisterAppliesCORS asserts the registry routes are wrapped in the
// supplied origin guard: a disallowed cross-origin request is rejected with 403
// before reaching the handler, so it cannot mutate the registry (CSRF defense).
func TestRegisterAppliesCORS(t *testing.T) {
	mdb := &memDB{}
	svc := New(mdb, Node{ID: "self", Name: "this", Source: SourceLocal, Self: true}, "", nil)
	// Empty tailnet suffix → loopback-only policy, so http://evil.com is denied.
	allowHost := func(string) bool { return true } // isolate origin behavior from the Host gate
	cors := func(next http.Handler) http.Handler { return api.CORS(allowHost, api.NewCORSPolicy(""), next) }
	h := NewHandlers(svc, newCounter(), cors)
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
	svc := New(mdb, Node{ID: "self", Name: "this", Source: SourceLocal, Self: true}, "", nil)
	allowHost := func(string) bool { return true } // isolate origin behavior from the Host gate
	cors := func(next http.Handler) http.Handler { return api.CORS(allowHost, api.NewCORSPolicy(""), next) }
	h := NewHandlers(svc, newCounter(), cors)
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

// TestAddRejectsOutOfRangePort asserts ports outside 1–65535 are rejected.
// url.Parse already rejects non-numeric/negative ports; these numeric-but-
// invalid ports would otherwise be saved and never connect.
func TestAddRejectsOutOfRangePort(t *testing.T) {
	for _, u := range []string{"http://gpu:0", "http://gpu:65536", "http://gpu:99999"} {
		h, _ := newTestHandlers()
		mux := http.NewServeMux()
		h.Register(mux)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
			strings.NewReader(`{"name":"x","url":"`+u+`"}`)))
		if rec.Code != 400 {
			t.Errorf("url %s: status = %d, want 400", u, rec.Code)
		}
	}
}

// TestAddRejectsSelfURL asserts a manual add whose url normalizes to the local
// node's dedup key is rejected with 409 rather than silently dropped by List's
// local-precedence dedup (the "I added it and it vanished" bug).
func TestAddRejectsSelfURL(t *testing.T) {
	mdb := &memDB{}
	self := Node{ID: "local", Name: "this", Source: SourceLocal, Self: true}
	// self-host:80 is this node's canonical origin (normalizes to http://self-host).
	svc := New(mdb, self, "http://self-host:80", nil)
	h := NewHandlers(svc, newCounter(), nil)
	mux := http.NewServeMux()
	h.Register(mux)

	// A bare and a port/case variant of the self origin both collapse to the
	// dedup key, so both must be rejected up front.
	for _, u := range []string{"http://self-host", "http://SELF-HOST:80"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
			strings.NewReader(`{"name":"me","url":"`+u+`"}`)))
		if rec.Code != 409 {
			t.Errorf("self-url %s: status = %d, want 409; body=%s", u, rec.Code, rec.Body.String())
		}
	}
	if len(mdb.nodes) != 0 {
		t.Errorf("self-url persisted despite 409: %+v", mdb.nodes)
	}
}

// TestUpdateRejectsSelfURL asserts the update path also rejects re-pointing a
// node at this node's own origin with 409 (the same self-origin guard as add).
func TestUpdateRejectsSelfURL(t *testing.T) {
	mdb := &memDB{}
	self := Node{ID: "local", Name: "this", Source: SourceLocal, Self: true}
	svc := New(mdb, self, "http://self-host:80", nil)
	h := NewHandlers(svc, newCounter(), nil)
	mux := http.NewServeMux()
	h.Register(mux)

	// Add a normal node, then try to PATCH it onto the self origin.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu:80"}`)))
	if rec.Code != 201 {
		t.Fatalf("add status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/id-1",
		strings.NewReader(`{"name":"me","url":"http://self-host"}`)))
	if rec.Code != 409 {
		t.Errorf("update to self-url status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMissingReturns404(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/nonexistent",
		strings.NewReader(`{"name":"new-name","url":"http://gpu:80"}`)))
	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestUpdateRequiresURL asserts the collapsed update path rejects a name-only
// PATCH (the rename-only branch was removed): both name and url are required.
func TestUpdateRequiresURL(t *testing.T) {
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
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/id-1",
		strings.NewReader(`{"name":"gpu-renamed"}`)))
	if rec.Code != 400 {
		t.Errorf("name-only update status = %d, want 400", rec.Code)
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

	// Rename it (url required, kept the same).
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/id-1",
		strings.NewReader(`{"name":"gpu-renamed","url":"http://gpu:80"}`)))
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

// TestUpdateEditsNameAndURL covers the edit path: a PATCH carrying a url updates
// the node's origin in place (normalized), keeping the same id.
func TestUpdateEditsNameAndURL(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu:80"}`)))
	if rec.Code != 201 {
		t.Fatalf("add status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Edit both name and host. The trailing slash and default port normalize away.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/id-1",
		strings.NewReader(`{"name":"workstation","url":"http://workstation/"}`)))
	if rec.Code != 204 {
		t.Fatalf("update status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nodes", nil))
	var body struct{ Nodes []Node }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	var gpu *Node
	for i := range body.Nodes {
		if body.Nodes[i].ID == "id-1" {
			gpu = &body.Nodes[i]
		}
	}
	if gpu == nil {
		t.Fatalf("edited node id-1 missing from %+v", body.Nodes)
	}
	if gpu.Name != "workstation" || gpu.URL != "http://workstation" {
		t.Errorf("after edit: name=%q url=%q, want workstation / http://workstation", gpu.Name, gpu.URL)
	}
}

// TestUpdateToExistingURLConflicts asserts that re-pointing a node at another
// node's url is rejected with 409 (the UNIQUE(url) constraint).
func TestUpdateToExistingURLConflicts(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	for _, u := range []string{"http://gpu:80", "http://tpu:80"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
			strings.NewReader(`{"name":"n","url":"`+u+`"}`)))
		if rec.Code != 201 {
			t.Fatalf("add %s status = %d", u, rec.Code)
		}
	}

	// id-2 (tpu) → gpu's url collides.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PATCH", "/api/nodes/id-2",
		strings.NewReader(`{"name":"tpu","url":"http://gpu"}`)))
	if rec.Code != 409 {
		t.Errorf("conflicting update status = %d, want 409", rec.Code)
	}
}
