package shared

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

const base36Chars = "0123456789abcdefghijklmnopqrstuvwxyz"

// GenerateID creates an ID with format: {prefix}_{timestamp36}_{random}.
func GenerateID(prefix string) string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	random := randomBase36(6)
	return fmt.Sprintf("%s_%s_%s", prefix, ts, random)
}

func randomBase36(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(36))
		if err != nil {
			panic("shared: crypto/rand.Int failed: " + err.Error())
		}
		b.WriteByte(base36Chars[idx.Int64()])
	}
	return b.String()
}
