package tokens_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_Returns64HexCharsAndMatchingHash(t *testing.T) {
	token, hash, err := tokens.Generate()
	require.NoError(t, err)
	assert.Len(t, token, 64, "expected 32 bytes hex-encoded = 64 chars")

	raw, err := hex.DecodeString(token)
	require.NoError(t, err)
	expected := sha256.Sum256(raw)
	assert.Equal(t, expected[:], hash)
}

func TestGenerate_TokensAreUnique(t *testing.T) {
	a, _, err := tokens.Generate()
	require.NoError(t, err)
	b, _, err := tokens.Generate()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

func TestHash_ReturnsSameHashAsGenerate(t *testing.T) {
	token, originalHash, err := tokens.Generate()
	require.NoError(t, err)

	again, err := tokens.Hash(token)
	require.NoError(t, err)
	assert.Equal(t, originalHash, again)
}

func TestHash_RejectsInvalidToken(t *testing.T) {
	cases := []string{
		"",
		"too-short",
		"zz" + string(make([]byte, 62)),
	}
	for _, c := range cases {
		_, err := tokens.Hash(c)
		require.ErrorIs(t, err, tokens.ErrInvalidToken)
	}
}
