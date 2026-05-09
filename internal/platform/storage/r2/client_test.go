//go:build integration

package r2_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/platform/storage/r2"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_PutFetchDelete(t *testing.T) {
	conn := testutil.NewTestMinio(t)
	bucket := "marketplace-test"
	createBucket(t, conn, bucket)

	cfg := config.Storage{
		Endpoint:        conn.Endpoint,
		AccessKeyID:     conn.AccessKeyID,
		SecretAccessKey: conn.SecretAccessKey,
		Bucket:          bucket,
		Region:          "us-east-1",
		PublicBaseURL:   "http://example.com/" + bucket,
		UsePathStyle:    true,
	}

	client, err := r2.New(context.Background(), cfg)
	require.NoError(t, err)

	body := []byte("hello world")
	require.NoError(t, client.Put(context.Background(), "test/key.txt", bytes.NewReader(body), int64(len(body)), "text/plain"))

	got := fetch(t, conn, bucket, "test/key.txt")
	assert.Equal(t, body, got)

	require.NoError(t, client.Delete(context.Background(), "test/key.txt"))
}

func createBucket(t *testing.T, conn testutil.MinioConn, bucket string) {
	t.Helper()
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			conn.AccessKeyID, conn.SecretAccessKey, "",
		)),
	)
	require.NoError(t, err)
	cli := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(conn.Endpoint)
		o.UsePathStyle = true
	})
	_, err = cli.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
}

func fetch(t *testing.T, conn testutil.MinioConn, bucket, key string) []byte {
	t.Helper()
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			conn.AccessKeyID, conn.SecretAccessKey, "",
		)),
	)
	require.NoError(t, err)
	cli := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(conn.Endpoint)
		o.UsePathStyle = true
	})
	resp, err := cli.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body
}
