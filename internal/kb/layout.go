package kb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// LayoutFile pins galaxy positions so adding a theme never moves the ones the user already knows.
const LayoutFile = "layout.json"

// MaxCoord bounds pinned coordinates; the generated map spans a few thousand units.
const MaxCoord = 1e6

type Pin struct {
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	Rotation *float64 `json:"rotation,omitempty"`
}

func (p Pin) Valid() bool {
	ok := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= MaxCoord }
	return ok(p.X) && ok(p.Y) && (p.Rotation == nil || ok(*p.Rotation))
}

type Layout struct {
	Galaxies map[string]Pin `json:"galaxies"`
}

func readLayout(root string) (Layout, error) {
	src, err := os.ReadFile(filepath.Join(root, LayoutFile))
	if os.IsNotExist(err) {
		return Layout{}, nil
	}
	if err != nil {
		return Layout{}, err
	}
	var l Layout
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return Layout{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return l, nil
}

// WriteLayout replaces layout.json. Keys come out sorted, so a diff only shows pins that moved.
func WriteLayout(root string, l Layout) (string, error) {
	if l.Galaxies == nil {
		l.Galaxies = map[string]Pin{}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return "", fmt.Errorf("write layout: %w", err)
	}
	path := filepath.Join(root, LayoutFile)
	return path, writeAtomic(path, append(data, '\n'))
}
