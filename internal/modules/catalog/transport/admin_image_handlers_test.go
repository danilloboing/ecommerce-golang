package transport_test

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

func newJPEGBytes(t *testing.T) []byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, src, &jpeg.Options{Quality: 80}))
	return buf.Bytes()
}

func multipartUpload(t *testing.T, alt string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("altText", alt))
	require.NoError(t, w.WriteField("position", "0"))
	fw, err := w.CreateFormFile("file", "vestido.jpg")
	require.NoError(t, err)
	_, err = io.Copy(fw, bytes.NewReader(newJPEGBytes(t)))
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
	require.NotNil(t, resp.Variants)
	assert.Equal(t, "https://cdn/t", resp.Variants.Thumb)
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
