package registry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/bxnlabs/argus/internal/node/db"
)

// WriteDB is the persistence dependency for mutations (satisfied by *db.DB).
type WriteDB interface {
	DB
	AddManualNode(ctx context.Context, id, name, url string) error
	RenameManualNode(ctx context.Context, id, name string) error
	DeleteManualNode(ctx context.Context, id string) error
}

// Handlers serves the /api/nodes registry endpoints.
type Handlers struct {
	svc   *Service
	db    WriteDB
	newID func() string
}

// NewHandlers builds the registry handlers. newID generates manual-node IDs
// (inject a deterministic one in tests; use a UUID in production).
func NewHandlers(svc *Service, db WriteDB, newID func() string) *Handlers {
	return &Handlers{svc: svc, db: db, newID: newID}
}

// Register wires the routes onto the given mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes", h.list)
	mux.HandleFunc("POST /api/nodes", h.add)
	mux.HandleFunc("PATCH /api/nodes/{id}", h.rename)
	mux.HandleFunc("DELETE /api/nodes/{id}", h.delete)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.svc.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (h *Handlers) add(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.URL = strings.TrimRight(strings.TrimSpace(body.URL), "/")
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	u, err := url.Parse(body.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url must be an absolute http(s) URL"})
		return
	}
	id := h.newID()
	if err := h.db.AddManualNode(r.Context(), id, body.Name, body.URL); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) rename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if err := h.db.RenameManualNode(r.Context(), id, strings.TrimSpace(body.Name)); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.db.DeleteManualNode(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
