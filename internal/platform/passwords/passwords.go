// Package passwords hashes and verifies passwords using argon2id (PHC encoding).
package passwords

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Above OWASP 2024 minimum (m=19MiB, t=2, p=1) — we trade a
// little CPU for more memory pressure to harden against GPU/ASIC attackers.
const (
	memoryKiB uint32 = 64 * 1024 // 64 MiB
	timeIters uint32 = 1
	threads   uint8  = 4
	keyLen    uint32 = 32
	saltLen   int    = 16
)

// ErrInvalidEncoded indicates a malformed argon2id PHC string.
var ErrInvalidEncoded = errors.New("passwords: invalid encoded format")

// DummyHash is a pre-computed argon2id PHC string used by the login flow to
// keep latency uniform when the email is not found. It MUST never authenticate
// any real input — the source plaintext is intentionally non-meaningful.
var DummyHash string

func init() {
	encoded, err := Hash("dummy-password-not-real-DO-NOT-USE")
	if err != nil {
		panic(fmt.Sprintf("passwords: precompute dummy hash: %v", err))
	}
	DummyHash = encoded
}

// Hash returns a PHC-encoded argon2id hash of plain.
// Format: $argon2id$v=<version>$m=<mem>,t=<time>,p=<threads>$<salt-b64>$<hash-b64>
func Hash(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("passwords: read random salt: %w", err)
	}

	hash := argon2.IDKey([]byte(plain), salt, timeIters, memoryKiB, threads, keyLen)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memoryKiB, timeIters, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// Verify checks plain against a PHC-encoded argon2id string.
// Returns ErrInvalidEncoded if the string is not well-formed.
func Verify(plain, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// Parts: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidEncoded
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidEncoded
	}
	if version != argon2.Version {
		return false, ErrInvalidEncoded
	}

	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, ErrInvalidEncoded
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidEncoded
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidEncoded
	}

	candidate := argon2.IDKey([]byte(plain), salt, t, m, p, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, candidate) == 1, nil
}
