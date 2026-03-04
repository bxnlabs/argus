package cli

import "testing"

func TestCompressPath(t *testing.T) {
	// Full test coverage in internal/shared/pathutil_test.go.
	// Smoke test to verify delegation works.
	got := compressPath("/Users/jeevb/Workspace/repos/bxnlabs/argus", "/Users/jeevb", 30)
	want := "~/Workspace/.../bxnlabs/argus"
	if got != want {
		t.Errorf("compressPath() = %q, want %q", got, want)
	}
}
