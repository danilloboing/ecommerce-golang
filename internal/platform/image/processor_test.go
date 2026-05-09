package image_test

import (
	"bytes"
	stdimage "image"
	"image/color"
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
			src.Set(x, y, color.RGBA{
				R: uint8((x * 255) / w),
				G: uint8((y * 255) / h),
				B: 128,
				A: 255,
			})
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

	expectedSizes := map[string][2]int{
		"thumb":  {200, 250},
		"medium": {600, 800},
		"large":  {1200, 1600},
	}
	for _, v := range variants {
		assert.NotEmpty(t, v.Name)
		assert.Greater(t, len(v.JPEGBody), 0)
		size, ok := expectedSizes[v.Name]
		require.True(t, ok, "unexpected variant name %q", v.Name)
		assert.Equal(t, size[0], v.Width)
		assert.Equal(t, size[1], v.Height)

		// Decode to verify JPEG output is valid.
		decoded, err := jpeg.Decode(bytes.NewReader(v.JPEGBody))
		require.NoError(t, err)
		assert.Equal(t, size[0], decoded.Bounds().Dx())
		assert.Equal(t, size[1], decoded.Bounds().Dy())
	}
}

func TestProcessor_RejectsNonImage(t *testing.T) {
	p := processor.New()
	_, err := p.Generate(bytes.NewReader([]byte("not an image")))
	require.Error(t, err)
}
