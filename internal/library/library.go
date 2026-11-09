// Package library keeps the original books and documents next to the knowledge repo,
// outside git (ADR-0010). A fiche points to a file by path and sha256.
package library

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

var ErrUnsupported = errors.New("unsupported file type")

var allowed = map[string]bool{".pdf": true, ".epub": true, ".md": true, ".txt": true}

type Entry struct {
	File   string // relative to the knowledge root, slash separated: library/<slug>-<hash8>.pdf
	SHA256 string
	Size   int64
	Added  bool // false when the same content was already in the library
}

// Add copies src into root/library under a name derived from its title and content hash,
// and makes sure the knowledge repo ignores the library folder.
func Add(root, src string) (Entry, error) {
	ext := strings.ToLower(filepath.Ext(src))
	if !allowed[ext] {
		return Entry{}, fmt.Errorf("library add %s: %w (want pdf, epub, md or txt)", src, ErrUnsupported)
	}
	sum, size, err := hashFile(src)
	if err != nil {
		return Entry{}, fmt.Errorf("library add: %w", err)
	}
	slug := kb.Slug(strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)))
	if slug == "" {
		slug = "document"
	}
	rel := kb.LibraryDir + "/" + slug + "-" + sum[:8] + ext
	dst := filepath.Join(root, filepath.FromSlash(rel))
	e := Entry{File: rel, SHA256: sum, Size: size}
	if err := ensureIgnored(root); err != nil {
		return Entry{}, err
	}
	if have, _, err := hashFile(dst); err == nil && have == sum {
		return e, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Entry{}, fmt.Errorf("library add: %w", err)
	}
	if err := copyFile(src, dst); err != nil {
		return Entry{}, fmt.Errorf("library add: %w", err)
	}
	e.Added = true
	return e, nil
}

func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".add-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func ensureIgnored(root string) error {
	p := filepath.Join(root, ".gitignore")
	line := kb.LibraryDir + "/"
	if f, err := os.Open(p); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if t := strings.TrimSpace(sc.Text()); t == line || t == "/"+line || t == kb.LibraryDir {
				f.Close()
				return nil
			}
		}
		f.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("library: read .gitignore: %w", err)
	}
	b, _ := os.ReadFile(p)
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	b = append(b, []byte(line+"\n")...)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return fmt.Errorf("library: write .gitignore: %w", err)
	}
	return nil
}
