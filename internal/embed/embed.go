// Package embed turns text into vectors with a small model running in-process (ADR-0013).
package embed

import (
	"context"
	"math"
)

// Kind tells the model which side of a search a text is on: most retrieval models expect a
// different prefix for the query and for the documents it is compared to.
type Kind int

const (
	Query Kind = iota
	Document
)

type Embedder interface {
	// Embed returns one L2-normalised vector per text, all of length Dims().
	Embed(ctx context.Context, texts []string, kind Kind) ([][]float32, error)
	Model() string
	// Fingerprint changes whenever the same text would get a different vector (revision,
	// prefixes, dimensions), so cached vectors never mix two ways of embedding.
	Fingerprint() string
	Dims() int
}

// Normalize scales v to unit length in place, so a dot product is a cosine similarity.
func Normalize(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	if n == 0 {
		return v
	}
	inv := float32(1 / math.Sqrt(n))
	for i := range v {
		v[i] *= inv
	}
	return v
}

// Dot is the cosine similarity of two normalised vectors.
func Dot(a, b []float32) float32 {
	var d float32
	for i := range a {
		d += a[i] * b[i]
	}
	return d
}
