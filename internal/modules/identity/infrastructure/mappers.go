// Package infrastructure wires identity domain to Postgres via sqlc.
package infrastructure

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapUser(row queries.User) domain.User {
	return domain.User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		Name:            row.Name,
		Status:          domain.UserStatus(row.Status),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func mapAuthMethod(row queries.AuthMethod) domain.AuthMethod {
	return domain.AuthMethod{
		ID:              row.ID,
		UserID:          row.UserID,
		Provider:        domain.AuthProvider(row.Provider),
		PasswordHash:    row.PasswordHash,
		ProviderSubject: row.ProviderSubject,
		CreatedAt:       row.CreatedAt,
		LastUsedAt:      row.LastUsedAt,
	}
}

func mapEmailVerifyToken(row queries.EmailVerifyToken) domain.EmailVerifyToken {
	return domain.EmailVerifyToken{
		TokenHash:  row.TokenHash,
		UserID:     row.UserID,
		Email:      row.Email,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func mapPasswordResetToken(row queries.PasswordResetToken) domain.PasswordResetToken {
	return domain.PasswordResetToken{
		TokenHash:  row.TokenHash,
		UserID:     row.UserID,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
