package registry

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/bxnlabs/argus/internal/node/db"
)

const maxBodySize = 1 << 20 // 1 MB

// Handlers serves the /api/nodes registry endpoints. They are thin HTTP
// adapters: decode the request, delegate to Service for all validation and
// persistence, and map the returned error to a status code.
type Handlers struct {
	svc   *Service
	newID func() string
	cors  func(http.Handler) http.Handler
}

// NewHandlers builds the registry handlers. newID generates manual-node IDs
// (inject a deterministic one in tests; use a UUID in production). cors wraps
// every route with the same cross-origin guard the node API uses — these
// endpoints have no app auth and decode JSON regardless of Content-Type, so an
// unguarded route would let any page CSRF a node add/rename/delete. A nil cors
// means no wrapping (test convenience).
func NewHandlers(svc *Service, newID func() string, cors func(http.Handler) http.Handler) *Handlers {
	if cors == nil {
		cors = func(next http.Handler) http.Handler { return next }
	}
	return &Handlers{svc: svc, newID: newID, cors: cors}
}

// Register wires the routes onto the given mux. Each route is wrapped in the
// CORS guard; the mux still matches method+path natively (and sets {id}) before
// the guard runs, so path values reach the handler unchanged.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/nodes", h.cors(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/nodes", h.cors(http.HandlerFunc(h.add)))
	mux.Handle("PATCH /api/nodes/{id}", h.cors(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/nodes/{id}", h.cors(http.HandlerFunc(h.delete)))

	// Wrap OPTIONS explicitly so a CORS preflight hits the guard (allowed →
	// 204 + ACAO, disallowed → 403) instead of falling through to the SPA
	// catch-all. The method-specific routes above don't match OPTIONS, so
	// without these a preflight would reach the "/" handler. The inner 204 only
	// runs for an OPTIONS with no Origin, since api.CORS answers a real
	// preflight itself.
	preflight := h.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.Handle("OPTIONS /api/nodes", preflight)
	mux.Handle("OPTIONS /api/nodes/{id}", preflight)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeInternalError logs the raw error server-side and sends a generic
// message to the client so internal details are not exposed.
func writeInternalError(w http.ResponseWriter, err error) {
	log.Printf("registry: internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

// writeServiceError maps a Service error to its HTTP status + client message.
// Anything unrecognized is treated as an internal error.
func writeServiceError(w http.ResponseWriter, err error) {
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ve.Msg})
	case errors.Is(err, ErrSelfOrigin):
		writeJSON(w, http.StatusConflict, map[string]string{"error": ErrSelfOrigin.Error()})
	case errors.Is(err, db.ErrDuplicateURL):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a node with that url already exists"})
	case errors.Is(err, db.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
	default:
		writeInternalError(w, err)
	}
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.svc.List(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (h *Handlers) add(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	id := h.newID()
	if err := h.svc.AddManualNode(r.Context(), id, body.Name, body.URL); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) update(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := h.svc.UpdateManualNode(r.Context(), r.PathValue("id"), body.Name, body.URL); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteManualNode(r.Context(), r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
