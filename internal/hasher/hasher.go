package hasher

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// NormalizeAndHashSHA256 normalizes a string (trims whitespace, converts to lowercase)
// and then computes its SHA-256 hash.
func NormalizeAndHashSHA256(input string) string {
	// Simple normalization: trim whitespace and convert to lowercase
	normalized := strings.ToLower(strings.TrimSpace(input))

	// Compute SHA-256 hash
	hasher := sha256.New()
	hasher.Write([]byte(normalized))
	return hex.EncodeToString(hasher.Sum(nil))
}
