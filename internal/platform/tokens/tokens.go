// Package tokens generates and hashes opaque single-use tokens for email
// verification and password reset. Tokens are 32 random bytes hex-encoded;
// only their SHA-256 hash is stored in the database.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const tokenSize = 32

// ErrInvalidToken indicates a malformed or wrong-length token string.
var ErrInvalidToken = errors.New("tokens: invalid token")

// Generate returns a new random token (hex string, 64 chars) and its SHA-256 hash.
// Callers send the hex token via email and persist the hash for later lookup.
func Generate() (token string, hash []byte, err error) {
	raw := make([]byte, tokenSize)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("tokens: read random: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(raw), sum[:], nil
}

// Hash decodes a hex token and returns its SHA-256 hash.
// Returns ErrInvalidToken if decoding fails or length is wrong.
func Hash(token string) ([]byte, error) {
	raw, err := hex.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if len(raw) != tokenSize {
		return nil, ErrInvalidToken
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}
