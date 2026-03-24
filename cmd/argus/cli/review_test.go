package cli

import (
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/git/review"
)

func TestFormatReviewMarkdown(t *testing.T) {
	r := &review.Review{
		Head: "feat/auth-system",
		Base: "main",
		Body: &review.ReviewBody{
			Body:      "Auth looks mostly good, but token handling needs hardening",
			Submitted: true,
			CreatedAt: "2026-03-16T10:30:00Z",
		},
		Comments: []review.ReviewComment{
			{
				ID: "rc_1", File: "src/auth.ts",
				Line:      review.LineRange{From: 52, To: 52},
				Snippet:   "const TOKEN_EXPIRY = 1800;",
				Body:      "Token expiry should be 3600 not 1800",
				Submitted: true,
			},
			{
				ID: "rc_2", File: "src/auth.ts",
				Line:      review.LineRange{From: 12, To: 15},
				Snippet:   "function validateToken(token) {\n  if (!token) return false;\n}",
				Body:      "Missing signature check",
				Submitted: false, // Should be excluded
			},
			{
				ID: "rc_3", File: "src/utils.ts",
				Line:      review.LineRange{From: 1, To: 3},
				Snippet:   "import { hash } from 'crypto';",
				Body:      "Use a proper hashing library",
				Submitted: true,
			},
		},
	}

	output := formatReviewMarkdown(r)

	if !strings.Contains(output, "## Review") {
		t.Error("missing ## Review header")
	}
	if !strings.Contains(output, "Branch: feat/auth-system vs main") {
		t.Error("missing branch line")
	}
	if !strings.Contains(output, "Auth looks mostly good, but token handling needs hardening") {
		t.Error("missing body text")
	}
	if strings.Contains(output, "### General") {
		t.Error("should not have a ### General section")
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
	if !strings.Contains(output, "### src/utils.ts") {
		t.Error("missing file header for src/utils.ts")
	}
}

func TestFormatReviewMarkdown_Empty(t *testing.T) {
	r := &review.Review{
		Head:     "feat/test",
		Base:     "main",
		Comments: []review.ReviewComment{},
	}
	output := formatReviewMarkdown(r)
	if !strings.Contains(output, "No submitted review comments") {
		t.Error("expected empty message")
	}
}
