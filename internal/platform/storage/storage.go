// Package storage defines a provider-neutral object storage interface.
package storage

import (
	"context"
	"io"
)

// Storage is the port catalog services use to put/get/delete blobs.
type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	URL(key string) string
	Delete(ctx context.Context, key string) error
}
