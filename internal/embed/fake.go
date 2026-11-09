package embed

import (
	"context"
	"hash/fnv"
	"strings"
	"sync/atomic"
	"unicode"
)

// Fake is a deterministic bag-of-words embedder for tests: texts sharing words (or words declared
// as synonyms) get close vectors. It never needs a model file, so CI stays offline.
type Fake struct {
	Synonyms map[string]string
	// Texts counts embedded texts, so tests can see what the cache saved.
	Texts atomic.Int64
}

const fakeDims = 512

func (f *Fake) Model() string { return "fake" }
func (f *Fake) Dims() int     { return fakeDims }

func (f *Fake) Embed(ctx context.Context, texts []string, _ Kind) ([][]float32, error) {
	f.Texts.Add(int64(len(texts)))
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v := make([]float32, fakeDims)
		for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
			if s, ok := f.Synonyms[w]; ok {
				w = s
			}
			h := fnv.New32a()
			h.Write([]byte(w))
			v[h.Sum32()%fakeDims]++
		}
		out[i] = Normalize(v)
	}
	return out, nil
}
