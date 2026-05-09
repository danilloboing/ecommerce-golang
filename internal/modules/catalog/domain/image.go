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
