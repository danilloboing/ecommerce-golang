package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

// MinioConn captures the data needed to point an S3 SDK at a MinIO testcontainer.
type MinioConn struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
}

// NewTestMinio spins up a MinIO container and returns connection info.
func NewTestMinio(t *testing.T) MinioConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const user = "testaccess"
	const pass = "testsecret123"

	container, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-12-18T13-15-44Z",
		tcminio.WithUsername(user),
		tcminio.WithPassword(pass),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	endpoint, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	return MinioConn{
		Endpoint:        "http://" + endpoint,
		AccessKeyID:     user,
		SecretAccessKey: pass,
	}
}
