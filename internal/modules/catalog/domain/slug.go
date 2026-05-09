package domain

import (
	"regexp"
	"strings"

	"github.com/gosimple/slug"
)

// Slug is a URL-friendly identifier in lowercase kebab-case.
type Slug struct {
	value string
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// SlugFromTitle generates a slug from an arbitrary human title (handles
// accents, punctuation, casing). Returns ErrInvalidSlug for blank input.
func SlugFromTitle(title string) (Slug, error) {
	candidate := slug.Make(strings.TrimSpace(title))
	if candidate == "" {
		return Slug{}, ErrInvalidSlug
	}
	return ParseSlug(candidate)
}

// ParseSlug validates and wraps a pre-existing slug.
func ParseSlug(s string) (Slug, error) {
	if !slugPattern.MatchString(s) {
		return Slug{}, ErrInvalidSlug
	}
	return Slug{value: s}, nil
}

// String returns the slug as a string.
func (s Slug) String() string { return s.value }

// IsZero reports whether the Slug is the zero value.
func (s Slug) IsZero() bool { return s.value == "" }
