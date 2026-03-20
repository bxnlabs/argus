package cli

import (
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/comments"
)

func TestFormatCommentsMarkdown(t *testing.T) {
	cf := &comments.CommentsFile{
		Branch:     "feat/auth-system",
		BaseBranch: "main",
		Comments: []comments.Comment{
			{
				ID: "rc_1", File: "src/auth.ts",
				Line:      comments.LineRange{From: 52, To: 52},
				Snippet:   "const TOKEN_EXPIRY = 1800;",
				Body:      "Token expiry should be 3600 not 1800",
				Submitted: true,
			},
			{
				ID: "rc_2", File: "src/auth.ts",
				Line:      comments.LineRange{From: 12, To: 15},
				Snippet:   "function validateToken(token) {\n  if (!token) return false;\n}",
				Body:      "Missing signature check",
				Submitted: false, // Should be excluded
			},
			{
				ID: "rc_3", File: "src/utils.ts",
				Line:      comments.LineRange{From: 1, To: 3},
				Snippet:   "import { hash } from 'crypto';",
				Body:      "Use a proper hashing library",
				Submitted: true,
			},
		},
		GeneralComment: &comments.GeneralComment{
			Body:      "Auth looks mostly good, but token handling needs hardening",
			Submitted: true,
		},
	}

	output := formatCommentsMarkdown(cf)

	if !strings.Contains(output, "## Comments") {
		t.Error("missing ## Comments header")
	}
	if !strings.Contains(output, "Branch: feat/auth-system vs main") {
		t.Error("missing branch line")
	}
	if !strings.Contains(output, "### src/auth.ts") {
		t.Error("missing file header for src/auth.ts")
	}
	if !strings.Contains(output, "**Lines 52-52:**") {
		t.Error("missing line range")
	}
	if !strings.Contains(output, "> const TOKEN_EXPIRY = 1800;") {
		t.Error("missing quoted snippet")
	}
	if !strings.Contains(output, "Token expiry should be 3600") {
		t.Error("missing comment body")
	}
	if strings.Contains(output, "Missing signature check") {
		t.Error("draft comment should not appear in output")
	}
	if !strings.Contains(output, "### General") {
		t.Error("missing general section")
	}
	if !strings.Contains(output, "token handling needs hardening") {
		t.Error("missing general comment body")
	}
}

func TestFormatCommentsMarkdown_Empty(t *testing.T) {
	cf := &comments.CommentsFile{
		Branch:     "feat/test",
		BaseBranch: "main",
		Comments:   []comments.Comment{},
	}
	output := formatCommentsMarkdown(cf)
	if !strings.Contains(output, "No submitted comments") {
		t.Error("expected empty message")
	}
}
