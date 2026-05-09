# Phase 1c — Image Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add product image management to the catalog. Admin uploads JPEG/PNG via multipart, server validates + stores original on Cloudflare R2, generates 3 JPEG variants (thumb 200×250, medium 600×800, large 1200×1600), persists image rows tied to products. Public APIs return CDN-friendly URLs. Orphan cleanup runs as a daily river job.

**Architecture:** Storage as `internal/platform/storage/r2/` adapter behind a `Storage` port; image processing in `internal/modules/catalog/image/` (pure-Go via `disintegration/imaging`, no CGO so deploy stays portable). Upload endpoint lives under the existing admin module group with the same static API token middleware. River job worker is a new `cmd/worker` binary that drains job queues continuously.

**Tech Stack:** Go 1.23+, `aws-sdk-go-v2` (S3 SDK pointed at R2 endpoint), `disintegration/imaging`, `golang.org/x/image`, `riverqueue/river`, `testcontainers-go/modules/minio`, `chi`, `testify`.

**Reference:** Design spec at `docs/superpowers/specs/2026-05-08-marketplace-golang-design.md` sections 2.1, 2.6, 4.1, 5 (Phase 1), 6.

**Depends on:** Plan 1b complete (`v0.2.0-catalog` tag) — catalog domain, admin auth, repository, and module wiring already exist.

---

## Out of Scope (defer to Phase 2+)

- WebP/AVIF generation (rely on Cloudflare Polish free tier for now)
- Direct browser→R2 signed upload URLs (CORS + post-upload callback complexity)
- libvips/govips backend (revisit when >10k uploads/day or perf hot)
- Image cropping / face detection / smart resize
- Bulk re-encoding workflows (when picking new sizes later, run a one-off script)
- Storage encryption-at-rest customer keys (R2 already encrypts at rest)

---

## Sizing the Variants

| Variant | Dimensions | Use case | Quality |
|---|---|---|---|
| `thumb` | 200×250 (cover crop) | Catalog listing thumbnails | JPEG q=80 |
| `medium` | 600×800 (cover crop) | Detail page primary | JPEG q=85 |
| `large` | 1200×1600 (cover crop) | Zoom + retina | JPEG q=85 |
| `original` | as-uploaded | Source of truth, regenerate from this | original bytes |

Object key convention in R2:

```
products/<productID>/<imageID>/original.<ext>
products/<productID>/<imageID>/thumb.jpg
products/<productID>/<imageID>/medium.jpg
products/<productID>/<imageID>/large.jpg
```

---

## File Structure (created/modified by this plan)

```
marketplace-golang/
├── cmd/
│   ├── api/main.go                                # MODIFIED: wire storage + image service
│   └── worker/main.go                             # NEW: river worker entry point
├── internal/
│   ├── config/config.go                           # MODIFIED: add Storage section
│   ├── modules/catalog/
│   │   ├── domain/image.go                        # MODIFIED: ImageVariants value object
│   │   ├── domain/image_test.go                   # NEW
│   │   ├── application/
│   │   │   ├── image_service.go                   # NEW: upload + variants
│   │   │   ├── image_service_test.go              # NEW
│   │   │   └── ports.go                           # MODIFIED: add ImageStorage, ImageProcessor ports
│   │   ├── infrastructure/
│   │   │   ├── repository.go                      # MODIFIED: image variant URL columns
│   │   │   └── repository_test.go                 # MODIFIED
│   │   └── transport/
│   │       ├── admin_image_handlers.go            # NEW
│   │       └── admin_image_handlers_test.go       # NEW
│   ├── platform/
│   │   ├── storage/
│   │   │   ├── storage.go                         # NEW: Storage port
│   │   │   └── r2/
│   │   │       ├── client.go                      # NEW: R2 client (S3 SDK)
│   │   │       └── client_test.go                 # NEW
│   │   └── image/
│   │       ├── processor.go                       # NEW: pure-Go variant generator
│   │       └── processor_test.go                  # NEW
│   └── platform/queue/
│       ├── client.go                              # NEW: river client wrapper
│       └── client_test.go                         # NEW
├── db/
│   ├── migrations/
│   │   └── 20260509000001_image_variants.sql      # NEW: variant URL columns
│   └── queries/variants.sql                       # MODIFIED: add image variant fields
├── api/openapi.yaml                               # MODIFIED: image upload endpoints
└── tests/integration/image_e2e_test.go            # NEW: E2E upload test
```

Each file has one responsibility; storage is provider-pluggable (`Storage` port → `r2.Client` adapter), image processing is provider-pluggable (`ImageProcessor` port → `image.Processor` adapter).

---

## Task 1: Domain — ImageVariants value object

**Files:**
- Create: `internal/modules/catalog/domain/image.go` (extend existing Image struct)
- Create: `internal/modules/catalog/domain/image_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-structs-interfaces` — value object boundary
- `cc-skills-golang:golang-naming` — variant names as enum
- `cc-skills-golang:golang-stretchr-testify` — table-driven assertions

- [ ] **Step 1: Write failing test**

```go
// internal/modules/catalog/domain/image_test.go
package domain_test

import (
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageVariants_AllVariantsRequireURL(t *testing.T) {
	_, err := domain.NewImageVariants(domain.ImageVariantURLs{
		Original: "",
		Thumb:    "https://cdn.example/t.jpg",
		Medium:   "https://cdn.example/m.jpg",
		Large:    "https://cdn.example/l.jpg",
	})
	require.ErrorIs(t, err, domain.ErrInvalidImageVariants)
}

func TestImageVariants_HappyPath(t *testing.T) {
	v, err := domain.NewImageVariants(domain.ImageVariantURLs{
		Original: "https://cdn.example/o.jpg",
		Thumb:    "https://cdn.example/t.jpg",
		Medium:   "https://cdn.example/m.jpg",
		Large:    "https://cdn.example/l.jpg",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/t.jpg", v.URLs().Thumb)
}

func TestImageVariants_AttachToProduct(t *testing.T) {
	in := newValidProductInput(t)
	p, err := domain.NewProduct(in)
	require.NoError(t, err)

	urls := domain.ImageVariantURLs{
		Original: "https://cdn.example/o.jpg",
		Thumb:    "https://cdn.example/t.jpg",
		Medium:   "https://cdn.example/m.jpg",
		Large:    "https://cdn.example/l.jpg",
	}
	variants, err := domain.NewImageVariants(urls)
	require.NoError(t, err)

	img := domain.Image{
		ID:       uuid.New(),
		URL:      urls.Original,
		Variants: &variants,
		Position: 0,
		AltText:  "vestido azul",
	}
	require.NoError(t, p.AddImage(img))

	got := p.Images()
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Variants)
	assert.Equal(t, urls.Thumb, got[0].Variants.URLs().Thumb)
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/modules/catalog/domain/... -v -run ImageVariants
```

Expected: build error.

- [ ] **Step 3: Add `ErrInvalidImageVariants` and types**

Append to `internal/modules/catalog/domain/errors.go`:

```go
// ErrInvalidImageVariants is returned when one or more variant URLs are missing.
var ErrInvalidImageVariants = errors.New("catalog: invalid image variants")
```

Create `internal/modules/catalog/domain/image.go` (additions; the Image struct already exists in product.go — move the struct here for cohesion in a follow-up commit if desired):

```go
package domain

// ImageVariantURLs holds the four required variant URLs.
type ImageVariantURLs struct {
	Original string
	Thumb    string
	Medium   string
	Large    string
}

// ImageVariants wraps the URL set after validation.
type ImageVariants struct {
	urls ImageVariantURLs
}

// NewImageVariants validates that all four variant URLs are non-empty.
func NewImageVariants(urls ImageVariantURLs) (ImageVariants, error) {
	if urls.Original == "" || urls.Thumb == "" || urls.Medium == "" || urls.Large == "" {
		return ImageVariants{}, ErrInvalidImageVariants
	}
	return ImageVariants{urls: urls}, nil
}

// URLs returns the underlying URLs.
func (v ImageVariants) URLs() ImageVariantURLs { return v.urls }
```

Modify `Image` in `product.go` to add `Variants *ImageVariants` field. Update tests that construct `domain.Image` accordingly (they'll keep `Variants: nil` for back-compat).

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/catalog/domain/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/catalog/domain/
git commit -m "feat(catalog): add ImageVariants value object and Image.Variants field"
```

---

## Task 2: Migration — image variant URL columns

**Files:**
- Create: `db/migrations/20260509000001_image_variants.sql`

**Skills to consult:**
- `cc-skills-golang:golang-database` — schema evolution, NULLable for backfill

- [ ] **Step 1: Write migration**

```sql
-- 20260509000001_image_variants.sql
-- Image variants stored alongside the original URL for CDN delivery.
ALTER TABLE catalog_images
    ADD COLUMN url_thumb TEXT,
    ADD COLUMN url_medium TEXT,
    ADD COLUMN url_large TEXT,
    ADD COLUMN storage_key TEXT;

-- Existing rows have NULL variants; admin can re-trigger generation later.
-- New rows must populate variants (enforced at app layer; DB stays permissive
-- so we can backfill in Phase 2 if needed).
COMMENT ON COLUMN catalog_images.storage_key IS 'R2 object key prefix (e.g. products/{pid}/{iid})';
```

- [ ] **Step 2: Hash + apply**

```bash
/Users/danilloboing/go/bin/atlas migrate hash --dir file://db/migrations
DATABASE_URL="postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable" \
  /Users/danilloboing/go/bin/atlas migrate apply --env local
```

Verify:

```bash
docker exec deployments-postgres-1 psql -U marketplace -d marketplace \
  -c "\d catalog_images"
```

Expected: `url_thumb`, `url_medium`, `url_large`, `storage_key` columns visible.

- [ ] **Step 3: Commit**

```bash
git add db/migrations/
git commit -m "feat(db): add image variant URL columns to catalog_images"
```

---

## Task 3: sqlc queries — image variant fields

**Files:**
- Modify: `db/queries/variants.sql`
- Regenerate: `internal/platform/postgres/queries/variants.sql.go`

**Skills to consult:**
- `cc-skills-golang:golang-database` — sqlc nullable handling
- `cc-skills-golang:golang-naming` — query naming

- [ ] **Step 1: Update CreateImage query**

```sql
-- name: CreateImage :one
INSERT INTO catalog_images (id, product_id, variant_id, url, alt_text, position,
                             url_thumb, url_medium, url_large, storage_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;
```

- [ ] **Step 2: Add variant lookup query**

```sql
-- name: ListImagesByProductWithVariants :many
SELECT id, product_id, variant_id, url, alt_text, position,
       url_thumb, url_medium, url_large, storage_key, created_at
FROM catalog_images
WHERE product_id = $1
ORDER BY position, created_at;

-- name: DeleteImageByID :exec
DELETE FROM catalog_images WHERE id = $1;
```

- [ ] **Step 3: Regenerate**

```bash
/Users/danilloboing/go/bin/sqlc generate -f db/sqlc.yaml
go build ./...
```

Expected: build clean.

- [ ] **Step 4: Update mappers and repository**

In `internal/modules/catalog/infrastructure/mappers.go`, when reading images, attach variants if all variant fields populated:

```go
mappedImages := make([]domain.Image, 0, len(images))
for _, img := range images {
    di := domain.Image{
        ID:       img.ID,
        URL:      img.Url,
        Position: int(img.Position),
        AltText:  img.AltText,
    }
    if img.UrlThumb != nil && img.UrlMedium != nil && img.UrlLarge != nil {
        v, err := domain.NewImageVariants(domain.ImageVariantURLs{
            Original: img.Url,
            Thumb:    *img.UrlThumb,
            Medium:   *img.UrlMedium,
            Large:    *img.UrlLarge,
        })
        if err == nil {
            di.Variants = &v
        }
    }
    mappedImages = append(mappedImages, di)
}
```

In `Repository.persistChildren`, pass variant URLs and storage key:

```go
var thumb, medium, large, key *string
if img.Variants != nil {
    u := img.Variants.URLs()
    thumb, medium, large = &u.Thumb, &u.Medium, &u.Large
}
// storage_key is set by the image service; pass through if Image has it (extend
// domain.Image with StorageKey string field if not present).
```

> **Note:** add a `StorageKey string` field to `domain.Image` first to thread it through. Update `image_test.go` accordingly.

- [ ] **Step 5: Commit**

```bash
git add db/queries/ internal/platform/postgres/queries/ internal/modules/catalog/
git commit -m "feat(catalog): persist image variant URLs and storage key"
```

---

## Task 4: Storage port + R2 client

**Files:**
- Create: `internal/platform/storage/storage.go`
- Create: `internal/platform/storage/r2/client.go`
- Create: `internal/platform/storage/r2/client_test.go`
- Create: `internal/testutil/minio.go` (testcontainers helper)
- Modify: `internal/config/config.go` (add Storage section)

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — port/adapter
- `cc-skills-golang:golang-naming` — Storage method names
- `cc-skills-golang:golang-error-handling` — wrap S3 errors
- `cc-skills-golang:golang-testing` — testcontainers MinIO

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/aws/aws-sdk-go-v2/aws \
       github.com/aws/aws-sdk-go-v2/config \
       github.com/aws/aws-sdk-go-v2/credentials \
       github.com/aws/aws-sdk-go-v2/service/s3 \
       github.com/testcontainers/testcontainers-go/modules/minio
go mod tidy
```

- [ ] **Step 2: Define `Storage` port**

```go
// Package storage defines a provider-neutral object storage interface.
package storage

import (
	"context"
	"io"
)

// Object represents one stored blob.
type Object struct {
	Key         string
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// Storage is the port catalog services use to put/get/delete blobs.
type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	URL(key string) string
	Delete(ctx context.Context, key string) error
}
```

- [ ] **Step 3: Add Storage config**

In `internal/config/config.go`:

```go
// Storage holds object storage settings.
type Storage struct {
	Endpoint        string `env:"STORAGE_ENDPOINT,required,notEmpty"`
	AccessKeyID     string `env:"STORAGE_ACCESS_KEY_ID,required,notEmpty"`
	SecretAccessKey string `env:"STORAGE_SECRET_ACCESS_KEY,required,notEmpty"`
	Bucket          string `env:"STORAGE_BUCKET,required,notEmpty"`
	Region          string `env:"STORAGE_REGION" envDefault:"auto"`
	PublicBaseURL   string `env:"STORAGE_PUBLIC_BASE_URL,required,notEmpty"`
	UsePathStyle    bool   `env:"STORAGE_USE_PATH_STYLE" envDefault:"true"`
}
```

Add `Storage Storage` to the top-level `Config` struct. Update `.env.example` with the new keys (use placeholder values).

Add tests verifying the new fields parse correctly (extend `config_test.go`).

- [ ] **Step 4: Implement R2 client**

```go
// Package r2 adapts S3 SDK to Cloudflare R2 (also works with MinIO for tests).
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
	s3      *s3.Client
	bucket  string
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
```

- [ ] **Step 5: Add MinIO testcontainer helper**

```go
// internal/testutil/minio.go
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

	container, err := tcminio.Run(ctx, "minio/minio:RELEASE.2025-01-20T14-49-07Z",
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
```

- [ ] **Step 6: Write failing R2 integration test**

```go
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
```

- [ ] **Step 7: Run integration test**

```bash
go test -tags=integration -count=1 -timeout=10m ./internal/platform/storage/r2/...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/platform/storage/ internal/testutil/minio.go internal/config/ go.mod go.sum
git commit -m "feat(storage): add Storage port and R2/S3-compatible adapter"
```

---

## Task 5: Image processor (pure Go)

**Files:**
- Create: `internal/platform/image/processor.go`
- Create: `internal/platform/image/processor_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-popular-libraries` — disintegration/imaging, x/image
- `cc-skills-golang:golang-naming` — variant naming
- `cc-skills-golang:golang-performance` — preallocate buffers
- `cc-skills-golang:golang-stretchr-testify` — golden image comparison

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/disintegration/imaging
go mod tidy
```

- [ ] **Step 2: Write failing test**

```go
package image_test

import (
	"bytes"
	stdimage "image"
	"image/jpeg"
	"testing"

	processor "github.com/danilloboing/marketplace-golang/internal/platform/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	src := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.Set(x, y, stdimage.NewUniform(stdimage.Rectangle{}.Min).At(x, y))
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, src, &jpeg.Options{Quality: 85}))
	return buf.Bytes()
}

func TestProcessor_GenerateVariants(t *testing.T) {
	src := makeJPEG(t, 2000, 3000)

	p := processor.New()
	variants, err := p.Generate(bytes.NewReader(src))

	require.NoError(t, err)
	require.Len(t, variants, 3)

	for _, v := range variants {
		assert.NotEmpty(t, v.Name)
		assert.Greater(t, len(v.JPEGBody), 0)
		assert.Greater(t, v.Width, 0)
		assert.Greater(t, v.Height, 0)
	}
}

func TestProcessor_RejectsNonImage(t *testing.T) {
	p := processor.New()
	_, err := p.Generate(bytes.NewReader([]byte("not an image")))
	require.Error(t, err)
}
```

- [ ] **Step 3: Implement processor**

```go
// Package image generates resized JPEG variants from a source image.
package image

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"

	"github.com/disintegration/imaging"
)

// Variant identifies a generated size.
type Variant struct {
	Name     string // thumb, medium, large
	Width    int
	Height   int
	JPEGBody []byte
}

// Processor generates JPEG variants using pure Go (no CGO).
type Processor struct{}

// New builds a Processor.
func New() *Processor { return &Processor{} }

type spec struct {
	name    string
	width   int
	height  int
	quality int
}

var variantSpecs = []spec{
	{"thumb", 200, 250, 80},
	{"medium", 600, 800, 85},
	{"large", 1200, 1600, 85},
}

// Generate decodes the source image once and emits each configured variant.
func (p *Processor) Generate(src io.Reader) ([]Variant, error) {
	srcBytes, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("image: read source: %w", err)
	}

	srcImg, _, err := image.Decode(bytes.NewReader(srcBytes))
	if err != nil {
		return nil, fmt.Errorf("image: decode source: %w", err)
	}

	out := make([]Variant, 0, len(variantSpecs))
	for _, s := range variantSpecs {
		resized := imaging.Fill(srcImg, s.width, s.height, imaging.Center, imaging.Lanczos)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: s.quality}); err != nil {
			return nil, fmt.Errorf("image: encode %s: %w", s.name, err)
		}
		out = append(out, Variant{
			Name:     s.name,
			Width:    s.width,
			Height:   s.height,
			JPEGBody: append([]byte(nil), buf.Bytes()...),
		})
	}
	return out, nil
}
```

> Add `_ "image/jpeg"` and `_ "image/png"` blank imports if a top-level init file is needed; in this package, `image/jpeg` is imported and registers itself.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/platform/image/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/image/ go.mod go.sum
git commit -m "feat(image): add pure-Go JPEG variant processor (thumb/medium/large)"
```

---

## Task 6: Application — ImageService

**Files:**
- Create: `internal/modules/catalog/application/image_service.go`
- Create: `internal/modules/catalog/application/image_service_test.go`
- Modify: `internal/modules/catalog/application/ports.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — orchestration service
- `cc-skills-golang:golang-error-handling` — partial failure rollback
- `cc-skills-golang:golang-context` — propagate ctx into storage calls
- `cc-skills-golang:golang-stretchr-testify` — fake storage + processor

- [ ] **Step 1: Add ports**

```go
// internal/modules/catalog/application/ports.go (append)

import (
	"io"
	imagex "github.com/danilloboing/marketplace-golang/internal/platform/image"
)

// ImageStorage abstracts the storage backend for image bytes.
type ImageStorage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	URL(key string) string
	Delete(ctx context.Context, key string) error
}

// ImageProcessor abstracts variant generation.
type ImageProcessor interface {
	Generate(src io.Reader) ([]imagex.Variant, error)
}

// ImageRepository persists image rows tied to products.
type ImageRepository interface {
	AttachImage(ctx context.Context, productID uuid.UUID, img domain.Image) error
}
```

(Add `AttachImage` to `Repository` in `infrastructure/repository.go` — calls `CreateImage` with full variant fields.)

- [ ] **Step 2: Write failing test**

```go
package application_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	imagex "github.com/danilloboing/marketplace-golang/internal/platform/image"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStorage struct {
	puts    map[string][]byte
	deletes []string
}

func newFakeStorage() *fakeStorage { return &fakeStorage{puts: map[string][]byte{}} }

func (f *fakeStorage) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	data, _ := io.ReadAll(body)
	f.puts[key] = data
	return nil
}

func (f *fakeStorage) URL(key string) string { return "https://cdn.test/" + key }

func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.deletes = append(f.deletes, key)
	return nil
}

type fakeProcessor struct{}

func (fakeProcessor) Generate(_ io.Reader) ([]imagex.Variant, error) {
	return []imagex.Variant{
		{Name: "thumb", Width: 200, Height: 250, JPEGBody: []byte("t")},
		{Name: "medium", Width: 600, Height: 800, JPEGBody: []byte("m")},
		{Name: "large", Width: 1200, Height: 1600, JPEGBody: []byte("l")},
	}, nil
}

type fakeImageRepo struct {
	attached []domain.Image
}

func (f *fakeImageRepo) AttachImage(_ context.Context, _ uuid.UUID, img domain.Image) error {
	f.attached = append(f.attached, img)
	return nil
}

func TestImageService_Upload_StoresOriginalAndVariantsAndPersists(t *testing.T) {
	store := newFakeStorage()
	repo := &fakeImageRepo{}
	svc := application.NewImageService(store, fakeProcessor{}, repo)

	productID := uuid.New()
	got, err := svc.Upload(context.Background(), application.UploadImageInput{
		ProductID:   productID,
		Filename:    "vestido.jpg",
		ContentType: "image/jpeg",
		Body:        bytes.NewReader([]byte("originalbytes")),
		Size:        13,
		AltText:     "vestido azul",
		Position:    1,
	})
	require.NoError(t, err)

	assert.Equal(t, "vestido azul", got.AltText)
	assert.NotNil(t, got.Variants)
	assert.Len(t, store.puts, 4) // original + 3 variants
	assert.Len(t, repo.attached, 1)
	urls := got.Variants.URLs()
	assert.Contains(t, urls.Original, "original.jpg")
	assert.Contains(t, urls.Thumb, "thumb.jpg")
}
```

- [ ] **Step 3: Implement ImageService**

```go
package application

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	imagex "github.com/danilloboing/marketplace-golang/internal/platform/image"
)

// ImageService stores product images: it persists the original, generates JPEG
// variants, uploads everything to object storage, and persists the row.
type ImageService struct {
	storage   ImageStorage
	processor ImageProcessor
	repo      ImageRepository
}

// NewImageService builds an ImageService.
func NewImageService(s ImageStorage, p ImageProcessor, r ImageRepository) *ImageService {
	return &ImageService{storage: s, processor: p, repo: r}
}

// UploadImageInput captures everything needed for an upload call.
type UploadImageInput struct {
	ProductID   uuid.UUID
	Filename    string
	ContentType string
	Body        io.Reader
	Size        int64
	AltText     string
	Position    int
}

// Upload runs the full upload pipeline and returns the resulting domain Image.
func (s *ImageService) Upload(ctx context.Context, in UploadImageInput) (domain.Image, error) {
	original, err := io.ReadAll(in.Body)
	if err != nil {
		return domain.Image{}, fmt.Errorf("image service: read upload: %w", err)
	}
	if int64(len(original)) != in.Size && in.Size > 0 {
		// Trust the actual byte count over the header.
		in.Size = int64(len(original))
	}

	imageID := uuid.New()
	keyPrefix := path.Join("products", in.ProductID.String(), imageID.String())
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(in.Filename), "."))
	if ext == "" {
		ext = "jpg"
	}
	originalKey := path.Join(keyPrefix, "original."+ext)

	if err := s.storage.Put(ctx, originalKey, bytes.NewReader(original), in.Size, in.ContentType); err != nil {
		return domain.Image{}, err
	}

	variants, err := s.processor.Generate(bytes.NewReader(original))
	if err != nil {
		_ = s.storage.Delete(ctx, originalKey)
		return domain.Image{}, fmt.Errorf("image service: generate variants: %w", err)
	}

	urls := domain.ImageVariantURLs{Original: s.storage.URL(originalKey)}
	uploaded := []string{originalKey}

	for _, v := range variants {
		key := path.Join(keyPrefix, v.Name+".jpg")
		if err := s.storage.Put(ctx, key, bytes.NewReader(v.JPEGBody), int64(len(v.JPEGBody)), "image/jpeg"); err != nil {
			rollback(ctx, s.storage, uploaded)
			return domain.Image{}, err
		}
		uploaded = append(uploaded, key)
		switch v.Name {
		case "thumb":
			urls.Thumb = s.storage.URL(key)
		case "medium":
			urls.Medium = s.storage.URL(key)
		case "large":
			urls.Large = s.storage.URL(key)
		}
	}

	domainVariants, err := domain.NewImageVariants(urls)
	if err != nil {
		rollback(ctx, s.storage, uploaded)
		return domain.Image{}, err
	}

	img := domain.Image{
		ID:         imageID,
		URL:        urls.Original,
		Variants:   &domainVariants,
		Position:   in.Position,
		AltText:    in.AltText,
		StorageKey: keyPrefix,
	}

	if err := s.repo.AttachImage(ctx, in.ProductID, img); err != nil {
		rollback(ctx, s.storage, uploaded)
		return domain.Image{}, err
	}

	return img, nil
}

// Compile-time guard against a stray unused import.
var _ imagex.Variant

func rollback(ctx context.Context, s ImageStorage, keys []string) {
	for _, k := range keys {
		_ = s.Delete(ctx, k)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/catalog/application/... -v -run Image
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/catalog/application/ internal/modules/catalog/infrastructure/ internal/modules/catalog/domain/
git commit -m "feat(catalog): add ImageService for upload + variant pipeline"
```

---

## Task 7: Admin image upload handler

**Files:**
- Create: `internal/modules/catalog/transport/admin_image_handlers.go`
- Create: `internal/modules/catalog/transport/admin_image_handlers_test.go`
- Modify: `internal/modules/catalog/module.go` (mount the new route)

**Skills to consult:**
- `cc-skills-golang:golang-naming` — handler method names
- `cc-skills-golang:golang-security` — multipart size limits, content-type sniffing
- `cc-skills-golang:golang-context` — request-scoped ctx
- `cc-skills-golang:golang-stretchr-testify` — multipart body builder

- [ ] **Step 1: Write failing test**

```go
package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/transport"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubImageSvc struct {
	got application.UploadImageInput
}

func (s *stubImageSvc) Upload(_ context.Context, in application.UploadImageInput) (domain.Image, error) {
	s.got = in
	urls := domain.ImageVariantURLs{
		Original: "https://cdn/orig", Thumb: "https://cdn/t",
		Medium: "https://cdn/m", Large: "https://cdn/l",
	}
	v, _ := domain.NewImageVariants(urls)
	return domain.Image{
		ID: uuid.New(), URL: urls.Original, Variants: &v,
		AltText: in.AltText, Position: in.Position,
	}, nil
}

func multipartUpload(t *testing.T, alt string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("altText", alt))
	require.NoError(t, w.WriteField("position", "0"))
	fw, err := w.CreateFormFile("file", "vestido.jpg")
	require.NoError(t, err)
	_, err = io.Copy(fw, bytes.NewReader([]byte("fakeimagebytes")))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func TestAdminImageHandler_UploadReturns201(t *testing.T) {
	svc := &stubImageSvc{}
	h := transport.NewAdminImageHandler(svc)
	r := chi.NewRouter()
	h.RegisterAdminImageRoutes(r)

	productID := uuid.New().String()
	body, contentType := multipartUpload(t, "alt do produto")

	req := httptest.NewRequest(http.MethodPost, "/admin/products/"+productID+"/images", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp transport.ImageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.URL)
	assert.Equal(t, "alt do produto", svc.got.AltText)
}

func TestAdminImageHandler_RejectsBadProductID(t *testing.T) {
	svc := &stubImageSvc{}
	h := transport.NewAdminImageHandler(svc)
	r := chi.NewRouter()
	h.RegisterAdminImageRoutes(r)

	body, contentType := multipartUpload(t, "x")
	req := httptest.NewRequest(http.MethodPost, "/admin/products/not-uuid/images", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
```

- [ ] **Step 2: Implement handler**

```go
package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

const maxUploadBytes = 10 << 20 // 10 MiB

// ImageUploader is the slice of ImageService consumed by the handler.
type ImageUploader interface {
	Upload(ctx context.Context, in application.UploadImageInput) (domain.Image, error)
}

// AdminImageHandler exposes the image upload endpoint.
type AdminImageHandler struct {
	svc ImageUploader
}

// NewAdminImageHandler builds the handler.
func NewAdminImageHandler(svc ImageUploader) *AdminImageHandler {
	return &AdminImageHandler{svc: svc}
}

// RegisterAdminImageRoutes mounts admin image routes.
func (h *AdminImageHandler) RegisterAdminImageRoutes(r chi.Router) {
	r.Post("/admin/products/{id}/images", h.upload)
}

func (h *AdminImageHandler) upload(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.WriteError(w, domain.ErrInvalidProduct)
		return
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		responsex.WriteError(w, errors.Join(domain.ErrInvalidProduct, err))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		responsex.WriteError(w, errors.Join(domain.ErrInvalidProduct, err))
		return
	}
	defer file.Close()

	if header.Size > maxUploadBytes {
		responsex.WriteError(w, domain.ErrInvalidProduct)
		return
	}

	position, _ := strconv.Atoi(r.FormValue("position"))

	img, err := h.svc.Upload(r.Context(), application.UploadImageInput{
		ProductID:   productID,
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Body:        file,
		Size:        header.Size,
		AltText:     r.FormValue("altText"),
		Position:    position,
	})
	if err != nil {
		responsex.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toImageResponse(img))
}

func toImageResponse(img domain.Image) ImageResponse {
	return ImageResponse{
		ID:       img.ID.String(),
		URL:      img.URL,
		AltText:  img.AltText,
		Position: img.Position,
	}
}
```

> Reuse the existing `ImageResponse` from `responses.go`. If you want richer output (variants), extend `ImageResponse` with optional variant URLs and update `toImageResponse` accordingly.

- [ ] **Step 3: Wire into module**

In `internal/modules/catalog/module.go`, accept image service deps and mount the route inside the admin group:

```go
func New(pool *pgxpool.Pool, store storage.Storage, processor application.ImageProcessor, adminToken string) *Module {
    repo := infrastructure.New(pool)
    publicSvc := application.NewPublicService(repo, repo)
    adminSvc := application.NewAdminService(repo)
    imgSvc := application.NewImageService(store, processor, repo)

    return &Module{
        publicHandler:    transport.NewPublicHandler(publicSvc),
        adminHandler:     transport.NewAdminHandler(adminSvc),
        adminImageHandler: transport.NewAdminImageHandler(imgSvc),
        adminToken:       adminToken,
    }
}

func (m *Module) Mount(r chi.Router) {
    r.Group(func(public chi.Router) {
        m.publicHandler.RegisterPublicRoutes(public)
    })
    r.Group(func(admin chi.Router) {
        admin.Use(adminauth.RequireToken(m.adminToken))
        m.adminHandler.RegisterAdminRoutes(admin)
        m.adminImageHandler.RegisterAdminImageRoutes(admin)
    })
}
```

Update `cmd/api/main.go` to construct the storage client + processor:

```go
storeClient, err := r2.New(rootCtx, cfg.Storage)
if err != nil { return fmt.Errorf("connect storage: %w", err) }
processor := imagex.New()
catalogModule := catalog.New(pool, storeClient, processor, cfg.Admin.APIToken)
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/catalog/transport/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/catalog/transport/ internal/modules/catalog/module.go cmd/api/main.go
git commit -m "feat(catalog): add admin image upload endpoint with variants"
```

---

## Task 8: River queue client + cleanup job

**Files:**
- Create: `internal/platform/queue/client.go`
- Create: `internal/platform/queue/client_test.go`
- Create: `cmd/worker/main.go`
- Create: `internal/modules/catalog/jobs/cleanup_orphans.go`
- Create: `internal/modules/catalog/jobs/cleanup_orphans_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-popular-libraries` — riverqueue/river
- `cc-skills-golang:golang-naming` — Worker[Args] pattern from river
- `cc-skills-golang:golang-context` — long-running worker ctx
- `cc-skills-golang:golang-design-patterns` — graceful shutdown

- [ ] **Step 1: Add dependency**

```bash
go get github.com/riverqueue/river \
       github.com/riverqueue/river/riverdriver/riverpgxv5 \
       github.com/riverqueue/rivermigrate/cmd/river
go mod tidy
```

- [ ] **Step 2: Apply river's own schema migration**

River ships a CLI to install its tables (`river_job`, etc). Add a migration step:

```bash
go run github.com/riverqueue/rivermigrate/cmd/river migrate-up \
    --database-url "postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable"
```

Capture the produced SQL and add it as `db/migrations/20260509000002_river.sql` so Atlas owns it. Easiest path: dump river's migrations into our migrations dir manually (river docs publish the SQL). Hash + apply via atlas.

- [ ] **Step 3: Implement queue client**

```go
// Package queue wires the river job queue.
package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// New builds a river client backed by Postgres.
func New(pool *pgxpool.Pool, workers *river.Workers) (*river.Client[any], error) {
	cli, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: build client: %w", err)
	}
	return cli, nil
}

// MustStart starts the worker loop.
func MustStart(ctx context.Context, cli *river.Client[any]) error {
	if err := cli.Start(ctx); err != nil {
		return fmt.Errorf("queue: start: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Implement cleanup job**

```go
package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// CleanupOrphansArgs declares the job payload.
type CleanupOrphansArgs struct{}

// Kind returns the job type identifier.
func (CleanupOrphansArgs) Kind() string { return "catalog.cleanup_orphans" }

// CleanupOrphansWorker deletes catalog_images rows whose product no longer exists
// and whose creation time is older than 24h.
type CleanupOrphansWorker struct {
	river.WorkerDefaults[CleanupOrphansArgs]
	Pool *pgxpool.Pool
}

// Work runs the deletion.
func (w *CleanupOrphansWorker) Work(ctx context.Context, _ *river.Job[CleanupOrphansArgs]) error {
	if w.Pool == nil {
		return errors.New("cleanup_orphans: nil pool")
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	_, err := w.Pool.Exec(ctx, `
        DELETE FROM catalog_images
        WHERE created_at < $1
          AND product_id NOT IN (SELECT id FROM catalog_products)
    `, cutoff)
	return err
}
```

- [ ] **Step 5: Schedule the job + worker entrypoint**

In `cmd/worker/main.go`, mirror `cmd/api/main.go` but build the river client + workers and call `cli.Start(ctx)`. Schedule the cleanup via a periodic job:

```go
periodic := river.NewPeriodicJob(
    river.PeriodicInterval(24*time.Hour),
    func() (river.JobArgs, *river.InsertOpts) {
        return jobs.CleanupOrphansArgs{}, nil
    },
    nil,
)
```

Pass `periodic` in `river.Config.PeriodicJobs`.

- [ ] **Step 6: Tests**

Unit-test the worker against testcontainer Postgres. Use a small helper to insert orphan + non-orphan rows and assert the deletion shape.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/queue/ internal/modules/catalog/jobs/ cmd/worker/ db/migrations/ go.mod go.sum
git commit -m "feat(queue): add river queue client and orphan-cleanup periodic job"
```

---

## Task 9: E2E integration test for image upload

**Files:**
- Create: `tests/integration/image_e2e_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-testing` — multi-container test composition
- `cc-skills-golang:golang-stretchr-testify` — multipart assertion
- `cc-skills-golang:golang-context` — long-running ctx
- `cc-skills-golang:golang-safety` — defer Close, defer rollback

- [ ] **Step 1: Write E2E test**

The test:
1. spins up Postgres + Redis + MinIO testcontainers,
2. applies migrations + creates a MinIO bucket,
3. seeds one category and one product via the admin API,
4. POST a multipart image to `/admin/products/{id}/images`,
5. asserts the response includes a CDN URL,
6. fetches `/products/{slug}` and verifies `images[0].variants` is populated.

Build the API binary like Plan 1b's E2E (avoid `go run` orphan), set the storage env vars to the MinIO container, wait for `/ready`, then run the assertions.

- [ ] **Step 2: Run**

```bash
make test-integration
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add tests/integration/image_e2e_test.go
git commit -m "test(integration): add image upload E2E (admin upload + public detail)"
```

---

## Task 10: OpenAPI extension for image upload

**Files:**
- Modify: `api/openapi.yaml`

**Skills to consult:**
- `cc-skills-golang:golang-swagger` — multipart bodies in OpenAPI 3
- `cc-skills-golang:golang-naming` — schema names

- [ ] **Step 1: Add path**

```yaml
/admin/products/{id}/images:
  post:
    summary: Upload an image for a product (admin)
    operationId: adminUploadProductImage
    security: [ { bearerAuth: [] } ]
    parameters:
      - name: id
        in: path
        required: true
        schema: { type: string, format: uuid }
    requestBody:
      required: true
      content:
        multipart/form-data:
          schema:
            type: object
            properties:
              file:    { type: string, format: binary }
              altText: { type: string }
              position: { type: integer }
            required: [file]
    responses:
      "201":
        description: Uploaded
        content:
          application/json:
            schema: { $ref: "#/components/schemas/Image" }
```

Extend the `Image` schema with `variants: { $ref: "#/components/schemas/ImageVariants" }` (define `ImageVariants` mirroring the domain struct).

- [ ] **Step 2: Commit**

```bash
git add api/openapi.yaml
git commit -m "docs(api): extend OpenAPI spec with admin image upload endpoint"
```

---

## Task 11: Final lint + tag

**Files:**
- Modify: any files needed for lint clean-up
- Modify: `Makefile` (add `worker` target)
- Modify: `deployments/docker-compose.yml` (add MinIO service for local dev, optional)

**Skills to consult:**
- `cc-skills-golang:golang-lint`
- `cc-skills-golang:golang-modernize`
- `cc-skills-golang:golang-documentation`

- [ ] **Step 1: Run vet + tests**

```bash
go vet ./...
go test -race -count=1 ./...
go test -tags=integration -count=1 -timeout=15m ./...
```

Expected: clean and PASS.

- [ ] **Step 2: Tag**

```bash
git tag -a v0.3.0-images -m "Phase 1c: image upload + variants + R2 storage + cleanup job"
```

- [ ] **Step 3: Final verification**

```bash
git log --oneline -25
git tag -l
```

Expected: clean tree, three tags (`v0.1.0-bootstrap`, `v0.2.0-catalog`, `v0.3.0-images`).

---

## Spec Coverage Self-Review

| Spec section | Covered by tasks |
|---|---|
| 2.1 Stack core (R2, image processing, river) | T4, T5, T8 |
| 2.6 Storage / images (R2 + libvips alternative + CDN) | T4, T5, T6 |
| 4.1 Catalog domain — Image | T1, T3 |
| 5 Phase 1 — image upload + variants generated | T6, T7, T9 |
| 5 Phase 1 — DoD: imagens upload + variantes geradas | T6, T9 |
| 6.1 Security — multipart size limits, content-type checks | T7 |
| 6.2 Performance — pure-Go pipeline, single decode per upload | T5 |
| 6.6 12-factor — storage config via env, secrets via env | T4 |

**Out-of-scope for Plan 1c:**
- WebP/AVIF generation (Cloudflare Polish handles client-side)
- Direct browser→R2 signed upload URLs
- libvips/govips backend swap

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-08-phase-1c-image-upload.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
