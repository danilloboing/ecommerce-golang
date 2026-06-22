//go:build integration

package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
)

func TestAuthMethodRepository_InsertFindUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	users := infrastructure.NewUserRepository(pool)
	auths := infrastructure.NewAuthMethodRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	am, err := auths.InsertPassword(ctx, u.ID, "$argon2id$v=19$m=65536,t=1,p=4$abc$def")
	require.NoError(t, err)
	require.NotNil(t, am.PasswordHash)
	assert.Equal(t, domain.AuthProviderPassword, am.Provider)

	got, err := auths.FindForUser(ctx, u.ID, domain.AuthProviderPassword)
	require.NoError(t, err)
	assert.Equal(t, am.ID, got.ID)

	require.NoError(t, auths.UpdatePassword(ctx, u.ID, "$argon2id$v=19$m=65536,t=1,p=4$xxx$yyy"))
	updated, err := auths.FindForUser(ctx, u.ID, domain.AuthProviderPassword)
	require.NoError(t, err)
	require.NotNil(t, updated.PasswordHash)
	assert.NotEqual(t, *am.PasswordHash, *updated.PasswordHash)
}

func TestAuthMethodRepository_FindForUser_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	users := infrastructure.NewUserRepository(pool)
	auths := infrastructure.NewAuthMethodRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	_, err = auths.FindForUser(ctx, u.ID, domain.AuthProviderPassword)
	require.True(t, errors.Is(err, domain.ErrUserNotFound))
}
