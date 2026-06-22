package passwords_test

import (
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/passwords"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHash_ProducesArgon2idEncodedString(t *testing.T) {
	encoded, err := passwords.Hash("S3cretP@ss!")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=1,p=4$"),
		"expected PHC argon2id prefix, got %q", encoded)
}

func TestVerify_AcceptsCorrectPassword(t *testing.T) {
	encoded, err := passwords.Hash("S3cretP@ss!")
	require.NoError(t, err)

	ok, err := passwords.Verify("S3cretP@ss!", encoded)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerify_RejectsIncorrectPassword(t *testing.T) {
	encoded, err := passwords.Hash("S3cretP@ss!")
	require.NoError(t, err)

	ok, err := passwords.Verify("not-the-password", encoded)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerify_RejectsMalformedEncoded(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=64$",
		"$bcrypt$something",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := passwords.Verify("anything", c)
			require.ErrorIs(t, err, passwords.ErrInvalidEncoded)
		})
	}
}

func TestDummyHash_VerifiesAgainstSomePassword(t *testing.T) {
	require.NotEmpty(t, passwords.DummyHash)
	// Must be a valid PHC argon2id encoded string so Verify does not return ErrInvalidEncoded.
	_, err := passwords.Verify("anything", passwords.DummyHash)
	require.NoError(t, err)
}
