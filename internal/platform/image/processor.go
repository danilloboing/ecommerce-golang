// Package image generates resized JPEG variants from a source image.
package image

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif" // register decoders
	"image/jpeg"
	_ "image/png" // register decoders
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

// Generate decodes the source image once and emits each configured variant
// as a JPEG-encoded byte slice.
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
