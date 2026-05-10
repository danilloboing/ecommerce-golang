//go:build integration

package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
)

func TestUserRepository_InsertFindUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	repo := infrastructure.NewUserRepository(pool)

	u, err := repo.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, u.ID)
	assert.Equal(t, "ana@example.com", u.Email)
	assert.Equal(t, "Ana", u.Name)
	assert.False(t, u.IsEmailVerified())

	got, err := repo.FindByEmail(ctx, "ANA@example.com") // CITEXT case-insensitive
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)

	gotByID, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, gotByID.ID)

	require.NoError(t, repo.MarkEmailVerified(ctx, u.ID))
	verified, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, verified.IsEmailVerified())

	updated, err := repo.UpdateName(ctx, u.ID, "Ana Lima")
	require.NoError(t, err)
	assert.Equal(t, "Ana Lima", updated.Name)
}

func TestUserRepository_Insert_ReturnsErrEmailAlreadyTakenOnDuplicate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	repo := infrastructure.NewUserRepository(pool)

	_, err := repo.Insert(ctx, "dup@example.com", "X")
	require.NoError(t, err)
	_, err = repo.Insert(ctx, "DUP@example.com", "Y")
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrEmailAlreadyTaken))
}

func TestUserRepository_FindByID_ReturnsNotFoundForUnknown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	repo := infrastructure.NewUserRepository(pool)

	_, err := repo.FindByID(ctx, uuid.New())
	require.True(t, errors.Is(err, domain.ErrUserNotFound))
}
