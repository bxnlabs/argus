package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/node/git/review"
)

func TestReviewHandler_GetEmpty(t *testing.T) {
	repoDir := homeTempDir(t)
	h := &reviewHandler{projectDirOverride: t.TempDir()}
	req := httptest.NewRequest("GET",
		"/git/review?path="+repoDir+"&branch=feat/test&base=main",
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
			Line:    review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 1}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 1}},
			Snippet: "package main", Body: "Add copyright header",
			Submitted: true, CreatedAt: "2026-03-16T10:30:00Z",
		}},
	}
	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest("POST",
		"/git/review?path="+repoDir,
		bytes.NewReader(body))
	postW := httptest.NewRecorder()
	h.post(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("POST expected 200, got %d: %s", postW.Code, postW.Body.String())
	}
	getReq := httptest.NewRequest("GET",
		"/git/review?path="+repoDir+"&branch=feat/test&base=main",
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
		"/git/review?path="+repoDir,
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.post(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal, got %d", w.Code)
	}
}

func TestReviewHandler_PostRejectsInvalidSide(t *testing.T) {
	repoDir := homeTempDir(t)
	h := &reviewHandler{projectDirOverride: t.TempDir()}
	payload := review.Review{
		Head: "feat/test",
		Base: "main",
		Comments: []review.ReviewComment{{
			ID:   "rc_1",
			File: "main.go",
			Line: review.LineRange{
				From: review.DiffPosition{Side: "X", Line: 1},
				To:   review.DiffPosition{Side: "X", Line: 1},
			},
			Body: "Invalid side",
		}},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST",
		"/git/review?path="+repoDir,
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.post(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid side, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewHandler_PostRejectsNonPositiveLine(t *testing.T) {
	repoDir := homeTempDir(t)
	h := &reviewHandler{projectDirOverride: t.TempDir()}
	payload := review.Review{
		Head: "feat/test",
		Base: "main",
		Comments: []review.ReviewComment{{
			ID:   "rc_1",
			File: "main.go",
			Line: review.LineRange{
				From: review.DiffPosition{Side: review.DiffSideRight, Line: 0},
				To:   review.DiffPosition{Side: review.DiffSideRight, Line: 0},
			},
			Body: "Zero line",
		}},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST",
		"/git/review?path="+repoDir,
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.post(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for line=0, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewHandler_PostRejectsOldPathTraversal(t *testing.T) {
	repoDir := homeTempDir(t)
	h := &reviewHandler{projectDirOverride: t.TempDir()}
	payload := review.Review{
		Head: "feat/test",
		Base: "main",
		Comments: []review.ReviewComment{{
			ID:      "rc_1",
			File:    "main.go",
			OldPath: "../etc/passwd",
			Line: review.LineRange{
				From: review.DiffPosition{Side: review.DiffSideLeft, Line: 1},
				To:   review.DiffPosition{Side: review.DiffSideLeft, Line: 1},
			},
			Body: "OldPath traversal",
		}},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST",
		"/git/review?path="+repoDir,
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.post(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oldPath traversal, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewHandler_PostRejectsFromNotEqualTo(t *testing.T) {
	repoDir := homeTempDir(t)
	h := &reviewHandler{projectDirOverride: t.TempDir()}
	payload := review.Review{
		Head: "feat/test",
		Base: "main",
		Comments: []review.ReviewComment{{
			ID:   "rc_1",
			File: "main.go",
			Line: review.LineRange{
				From: review.DiffPosition{Side: review.DiffSideRight, Line: 1},
				To:   review.DiffPosition{Side: review.DiffSideRight, Line: 5},
			},
			Body: "From != to",
		}},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST",
		"/git/review?path="+repoDir,
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.post(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for from!=to, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewHandler_Delete(t *testing.T) {
	repoDir := homeTempDir(t)
	overrideDir := t.TempDir()
	h := &reviewHandler{projectDirOverride: overrideDir}
	payload := review.Review{
		Head: "feat/test",
		Base: "main",
		Comments: []review.ReviewComment{{
			ID:   "rc_1",
			File: "x.go",
			Line: review.LineRange{
				From: review.DiffPosition{Side: review.DiffSideRight, Line: 1},
				To:   review.DiffPosition{Side: review.DiffSideRight, Line: 1},
			},
			Body: "test",
		}},
	}
	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest("POST",
		"/git/review?path="+repoDir,
		bytes.NewReader(body))
	postW := httptest.NewRecorder()
	h.post(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("POST expected 200, got %d: %s", postW.Code, postW.Body.String())
	}
	delReq := httptest.NewRequest("DELETE",
		"/git/review?path="+repoDir+"&branch=feat/test&base=main",
		nil)
	delW := httptest.NewRecorder()
	h.delete(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Errorf("DELETE expected 200, got %d", delW.Code)
	}
}
