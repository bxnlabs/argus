package cli

import "github.com/bxnlabs/argus/internal/shared"

// compressPath delegates to the shared implementation.
func compressPath(path, home string, threshold int) string {
	return shared.CompressPath(path, home, threshold)
}
