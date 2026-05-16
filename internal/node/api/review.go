package api

import (
	"fmt"
	"net/http"

	"github.com/bxnlabs/argus/internal/node/git/review"
	"github.com/bxnlabs/argus/internal/shared"
)

func validateCommentLine(lr review.LineRange) error {
	if lr.From.Side != review.DiffSideLeft && lr.From.Side != review.DiffSideRight {
		return fmt.Errorf("invalid line.from.side: %q", lr.From.Side)
	}
	if lr.To.Side != review.DiffSideLeft && lr.To.Side != review.DiffSideRight {
		return fmt.Errorf("invalid line.to.side: %q", lr.To.Side)
	}
	if lr.From.Line <= 0 {
		return fmt.Errorf("line.from.line must be > 0")
	}
	if lr.To.Line <= 0 {
		return fmt.Errorf("line.to.line must be > 0")
	}
	if lr.From.Side != lr.To.Side || lr.From.Line != lr.To.Line {
		return fmt.Errorf("line.from must equal line.to (single-line comments only)")
	}
	return nil
}

type reviewHandler struct {
	projectDirOverride string // for testing — bypasses home dir derivation
}

func (h *reviewHandler) resolveProjectDir(expandedPath string) (string, error) {
	return resolveProjectDir(expandedPath, h.projectDirOverride)
}

// GET /api/git/review?path=...&branch=...&base=...&headRef=...&baseRef=...
func (h *reviewHandler) get(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	base := r.URL.Query().Get("base")
	if repoPath == "" || branch == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path, branch, and base parameters are required")
		return
	}
	headRef := r.URL.Query().Get("headRef")
	baseRef := r.URL.Query().Get("baseRef")
	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	rv, err := review.Load(projectDir, expandedPath, branch, base, headRef, baseRef)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if rv == nil {
		rv = &review.Review{
			Head:     branch,
			Base:     base,
			Comments: []review.ReviewComment{},
		}
	}
	respondJSON(w, http.StatusOK, rv)
}

// POST /api/git/review?path=...
func (h *reviewHandler) post(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	if repoPath == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	var rv review.Review
	if err := parseBody(w, r, &rv); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, c := range rv.Comments {
		if _, err := sanitizeFilePath(expandedPath, c.File); err != nil {
			respondError(w, http.StatusBadRequest, "invalid file path in comment: "+c.File)
			return
		}
		if c.OldPath != "" {
			if _, err := sanitizeFilePath(expandedPath, c.OldPath); err != nil {
				respondError(w, http.StatusBadRequest, "invalid oldPath in comment: "+c.OldPath)
				return
			}
		}
		if err := validateCommentLine(c.Line); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := review.Save(projectDir, &rv); err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/git/review?path=...&branch=...&base=...
func (h *reviewHandler) delete(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	base := r.URL.Query().Get("base")
	if repoPath == "" || branch == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path, branch, and base parameters are required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if err := review.Delete(projectDir, branch, base); err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
