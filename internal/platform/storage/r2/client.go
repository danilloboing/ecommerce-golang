// Package r2 adapts the AWS S3 SDK to Cloudflare R2 (also works with MinIO for tests).
package r2

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/platform/storage"
)

// Client is an S3-compatible client tuned for Cloudflare R2.
type Client struct {
	s3            *s3.Client
	bucket        string
	publicBaseURL string
}

// New constructs a client from configuration.
func New(ctx context.Context, cfg config.Storage) (*Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("r2: load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &Client{s3: s3Client, bucket: cfg.Bucket, publicBaseURL: cfg.PublicBaseURL}, nil
}

// Put uploads an object.
func (c *Client) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("r2: put %q: %w", key, err)
	}
	return nil
}

// URL returns the public CDN URL for an object key.
func (c *Client) URL(key string) string {
	return fmt.Sprintf("%s/%s", c.publicBaseURL, key)
}

// Delete removes an object.
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("r2: delete %q: %w", key, err)
	}
	return nil
}

// Compile-time interface check.
var _ storage.Storage = (*Client)(nil)
