package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/node/review"
)

func TestReviewHandler_GetEmpty(t *testing.T) {
	repoDir := homeTempDir(t)
	h := &reviewHandler{projectDirOverride: t.TempDir()}
	req := httptest.NewRequest("GET",
		"/api/git/review?path="+repoDir+"&branch=feat/test&base=main",
		nil)
	w := httptest.NewRecorder()
	h.get(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp review.Review
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Comments) != 0 {
		t.Errorf("expected empty comments, got %d", len(resp.Comments))
	}
}

func TestReviewHandler_PostAndGet(t *testing.T) {
	repoDir := homeTempDir(t)
	overrideDir := t.TempDir()
	os.MkdirAll(filepath.Join(repoDir, "src"), 0o755)
	os.WriteFile(filepath.Join(repoDir, "src", "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644)
	h := &reviewHandler{projectDirOverride: overrideDir}
	payload := review.Review{
		Head: "feat/test",
		Base: "main",
		Comments: []review.ReviewComment{{
			ID: "rc_1_abc", File: "src/main.go",
			Line: review.LineRange{From: 1, To: 1},
			Snippet: "package main", Body: "Add copyright header",
			Submitted: true, CreatedAt: "2026-03-16T10:30:00Z",
		}},
	}
	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest("POST",
		"/api/git/review?path="+repoDir,
		bytes.NewReader(body))
	postW := httptest.NewRecorder()
	h.post(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("POST expected 200, got %d: %s", postW.Code, postW.Body.String())
	}
	getReq := httptest.NewRequest("GET",
		"/api/git/review?path="+repoDir+"&branch=feat/test&base=main",
		nil)
	getW := httptest.NewRecorder()
	h.get(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", getW.Code)
	}
	var resp review.Review
	json.Unmarshal(getW.Body.Bytes(), &resp)
	if len(resp.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(resp.Comments))
	}
}

func TestReviewHandler_PostRejectsTraversal(t *testing.T) {
	repoDir := homeTempDir(t)
	h := &reviewHandler{projectDirOverride: t.TempDir()}
	payload := review.Review{
		Head: "feat/test",
		Base: "main",
		Comments: []review.ReviewComment{{
			ID: "rc_1", File: "../etc/passwd",
			Body: "Traversal attempt",
		}},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST",
		"/api/git/review?path="+repoDir,
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.post(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal, got %d", w.Code)
	}
}

func TestReviewHandler_Delete(t *testing.T) {
	repoDir := homeTempDir(t)
	overrideDir := t.TempDir()
	h := &reviewHandler{projectDirOverride: overrideDir}
	payload := review.Review{
		Head:     "feat/test",
		Base:     "main",
		Comments: []review.ReviewComment{{ID: "rc_1", File: "x.go", Body: "test"}},
	}
	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest("POST",
		"/api/git/review?path="+repoDir,
		bytes.NewReader(body))
	postW := httptest.NewRecorder()
	h.post(postW, postReq)
	delReq := httptest.NewRequest("DELETE",
		"/api/git/review?path="+repoDir+"&branch=feat/test&base=main",
		nil)
	delW := httptest.NewRecorder()
	h.delete(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Errorf("DELETE expected 200, got %d", delW.Code)
	}
}
