//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageE2E_AdminUploadAndPublicDetail(t *testing.T) {
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	addr := testutil.NewTestRedisAddr(t)
	minio := testutil.NewTestMinio(t)
	bucket := "marketplace-test"
	createBucket(t, minio, bucket)

	port := "18093"

	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bin := testutil.BuildAPIBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"APP_PORT="+port,
		"APP_ENV=test",
		"APP_LOG_LEVEL=warn",
		"DATABASE_URL="+dsn,
		"REDIS_ADDR="+addr,
		"ADMIN_API_TOKEN=secret",
		"CORS_ALLOWED_ORIGINS=http://test.local",
		"STORAGE_ENDPOINT="+minio.Endpoint,
		"STORAGE_ACCESS_KEY_ID="+minio.AccessKeyID,
		"STORAGE_SECRET_ACCESS_KEY="+minio.SecretAccessKey,
		"STORAGE_BUCKET="+bucket,
		"STORAGE_REGION=us-east-1",
		"STORAGE_PUBLIC_BASE_URL="+minio.Endpoint+"/"+bucket,
		"STORAGE_USE_PATH_STYLE=true",
		"EMAIL_VERIFY_LINK_BASE_URL=http://test.local/verify",
		"EMAIL_RESET_LINK_BASE_URL=http://test.local/reset",
	)
	var logBuf bytes.Buffer
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGINT)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		if t.Failed() {
			t.Logf("=== api logs ===\n%s\n=== end ===", logBuf.String())
		}
	})

	base := "http://127.0.0.1:" + port
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 200*time.Millisecond)

	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/ready")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 15*time.Second, 200*time.Millisecond)

	// Create category + product.
	resp := postJSONImage(t, base+"/admin/categories", "secret", map[string]any{
		"slug": "vestidos", "name": "Vestidos",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var catBody map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&catBody))
	resp.Body.Close()
	categoryID, err := uuid.Parse(catBody["id"])
	require.NoError(t, err)

	resp = postJSONImage(t, base+"/admin/products", "secret", map[string]any{
		"slug":           "vestido-azul",
		"name":           "Vestido Azul",
		"description":    "x",
		"brand":          "AcmeFashion",
		"categoryId":     categoryID.String(),
		"basePriceCents": 9990,
		"currency":       "BRL",
		"status":         "published",
		"variants": []map[string]any{
			{"sku": "VA-P", "size": "P", "color": "Azul"},
		},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var prodBody struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&prodBody))
	resp.Body.Close()

	// Multipart upload of a real JPEG.
	body, ct := multipartImageUpload(t, "vestido azul")
	req, err := http.NewRequest(http.MethodPost, base+"/admin/products/"+prodBody.ID+"/images", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", ct)
	uploadResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusCreated {
		uploadBody, _ := io.ReadAll(uploadResp.Body)
		t.Logf("upload response body: %s", string(uploadBody))
	}
	require.Equal(t, http.StatusCreated, uploadResp.StatusCode)

	var uploaded map[string]any
	require.NoError(t, json.NewDecoder(uploadResp.Body).Decode(&uploaded))
	variants, ok := uploaded["variants"].(map[string]any)
	require.True(t, ok, "variants must be present in upload response")
	assert.NotEmpty(t, variants["thumb"])
	assert.NotEmpty(t, variants["medium"])
	assert.NotEmpty(t, variants["large"])
	assert.NotEmpty(t, variants["original"])

	// Public detail returns the image with variants.
	detail, err := http.Get(base + "/products/vestido-azul")
	require.NoError(t, err)
	defer detail.Body.Close()
	require.Equal(t, http.StatusOK, detail.StatusCode)
	detailBody, err := io.ReadAll(detail.Body)
	require.NoError(t, err)
	assert.Contains(t, string(detailBody), `"variants"`)
	assert.Contains(t, string(detailBody), `"thumb"`)
}

func multipartImageUpload(t *testing.T, alt string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("altText", alt))
	require.NoError(t, w.WriteField("position", "0"))
	fw, err := w.CreateFormFile("file", "vestido.jpg")
	require.NoError(t, err)

	// Build a real 1500x2000 JPEG so the processor has something meaningful to resize.
	src := image.NewRGBA(image.Rect(0, 0, 1500, 2000))
	for y := 0; y < 2000; y++ {
		for x := 0; x < 1500; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 100, A: 255})
		}
	}
	require.NoError(t, jpeg.Encode(fw, src, &jpeg.Options{Quality: 80}))
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func postJSONImage(t *testing.T, url, token string, payload map[string]any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
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
	if err != nil && !strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") {
		require.NoError(t, err)
	}
}
