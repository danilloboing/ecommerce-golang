package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	imagex "github.com/danilloboing/marketplace-golang/internal/platform/image"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStorage struct {
	puts      map[string][]byte
	deletes   []string
	failOnKey string
}

func newFakeStorage() *fakeStorage { return &fakeStorage{puts: map[string][]byte{}} }

func (f *fakeStorage) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	if f.failOnKey != "" && strings.Contains(key, f.failOnKey) {
		return errors.New("storage put failed")
	}
	data, _ := io.ReadAll(body)
	f.puts[key] = data
	return nil
}

func (f *fakeStorage) URL(key string) string { return "https://cdn.test/" + key }

func (f *fakeStorage) Delete(_ context.Context, key string) error {
	delete(f.puts, key)
	f.deletes = append(f.deletes, key)
	return nil
}

type fakeProcessor struct{ err error }

func (p fakeProcessor) Generate(_ io.Reader) ([]imagex.Variant, error) {
	if p.err != nil {
		return nil, p.err
	}
	return []imagex.Variant{
		{Name: "thumb", Width: 200, Height: 250, JPEGBody: []byte("t")},
		{Name: "medium", Width: 600, Height: 800, JPEGBody: []byte("m")},
		{Name: "large", Width: 1200, Height: 1600, JPEGBody: []byte("l")},
	}, nil
}

type fakeImageRepo struct {
	attached []domain.Image
	err      error
}

func (f *fakeImageRepo) AttachImage(_ context.Context, _ uuid.UUID, img domain.Image) error {
	if f.err != nil {
		return f.err
	}
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
	require.NotNil(t, got.Variants)
	assert.Len(t, store.puts, 4)
	assert.Len(t, repo.attached, 1)
	urls := got.Variants.URLs()
	assert.Contains(t, urls.Original, "original.jpg")
	assert.Contains(t, urls.Thumb, "thumb.jpg")
	assert.Contains(t, urls.Medium, "medium.jpg")
	assert.Contains(t, urls.Large, "large.jpg")
}

func TestImageService_Upload_RollsBackOnVariantPutFailure(t *testing.T) {
	store := newFakeStorage()
	store.failOnKey = "medium.jpg"
	repo := &fakeImageRepo{}
	svc := application.NewImageService(store, fakeProcessor{}, repo)

	_, err := svc.Upload(context.Background(), application.UploadImageInput{
		ProductID:   uuid.New(),
		Filename:    "x.jpg",
		ContentType: "image/jpeg",
		Body:        bytes.NewReader([]byte("bytes")),
		Size:        5,
	})
	require.Error(t, err)

	assert.Empty(t, store.puts, "all uploaded objects should have been rolled back")
	assert.Empty(t, repo.attached)
}

func TestImageService_Upload_RollsBackOnRepoFailure(t *testing.T) {
	store := newFakeStorage()
	repo := &fakeImageRepo{err: errors.New("db failed")}
	svc := application.NewImageService(store, fakeProcessor{}, repo)

	_, err := svc.Upload(context.Background(), application.UploadImageInput{
		ProductID:   uuid.New(),
		Filename:    "x.jpg",
		ContentType: "image/jpeg",
		Body:        bytes.NewReader([]byte("bytes")),
		Size:        5,
	})
	require.Error(t, err)
	assert.Empty(t, store.puts, "objects should be deleted after repo failure")
}

func TestImageService_Upload_RollsBackOnProcessorFailure(t *testing.T) {
	store := newFakeStorage()
	repo := &fakeImageRepo{}
	svc := application.NewImageService(store, fakeProcessor{err: errors.New("decode failed")}, repo)

	_, err := svc.Upload(context.Background(), application.UploadImageInput{
		ProductID:   uuid.New(),
		Filename:    "x.jpg",
		ContentType: "image/jpeg",
		Body:        bytes.NewReader([]byte("bytes")),
		Size:        5,
	})
	require.Error(t, err)
	assert.Empty(t, store.puts, "original should be removed after processor failure")
}
