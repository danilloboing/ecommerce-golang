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
// On any failure after object writes start, previously uploaded blobs are
// best-effort deleted so storage stays clean.
func (s *ImageService) Upload(ctx context.Context, in UploadImageInput) (domain.Image, error) {
	original, err := io.ReadAll(in.Body)
	if err != nil {
		return domain.Image{}, fmt.Errorf("image service: read upload: %w", err)
	}
	if int64(len(original)) != in.Size && in.Size > 0 {
		in.Size = int64(len(original))
	}
	if in.Size == 0 {
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

func rollback(ctx context.Context, s ImageStorage, keys []string) {
	for _, k := range keys {
		_ = s.Delete(ctx, k)
	}
}
