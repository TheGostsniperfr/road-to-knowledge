package kb

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// LibraryDir holds the original books and documents. It is git-ignored (ADR-0010): the
// fiche keeps the hash, so the file can be found again on another machine.
const LibraryDir = "library"

// Source is the work a fiche was written from: a book, a long PDF, a course.
type Source struct {
	Title  string `yaml:"title" json:"title"`
	Author string `yaml:"author,omitempty" json:"author,omitempty"`
	Year   int    `yaml:"year,omitempty" json:"year,omitempty"`
	Pages  int    `yaml:"pages,omitempty" json:"pages,omitempty"`
	File   string `yaml:"file,omitempty" json:"file,omitempty"`
	SHA256 string `yaml:"sha256,omitempty" json:"sha256,omitempty"`
	URL    string `yaml:"url,omitempty" json:"url,omitempty"`
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Problems lists what is wrong with a source; an empty list means it is usable.
func (s Source) Problems() []string {
	var out []string
	if strings.TrimSpace(s.Title) == "" {
		out = append(out, "source needs a title")
	}
	if s.File != "" && !ValidLibraryPath(s.File) {
		out = append(out, "source file must be a relative path inside library/, got "+s.File)
	}
	if s.SHA256 != "" && !sha256Hex.MatchString(s.SHA256) {
		out = append(out, "source sha256 must be 64 lowercase hex characters")
	}
	if s.Year < 0 || s.Pages < 0 {
		out = append(out, "source year and pages cannot be negative")
	}
	return out
}

func ValidLibraryPath(p string) bool {
	if p == "" || strings.Contains(p, "\\") || path.IsAbs(p) {
		return false
	}
	if path.Clean(p) != p || !strings.HasPrefix(p, LibraryDir+"/") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return false
		}
	}
	return true
}

// Available reports whether the source file is on this machine.
func (s Source) Available(root string) bool {
	if s.File == "" || !ValidLibraryPath(s.File) {
		return false
	}
	st, err := os.Stat(filepath.Join(root, filepath.FromSlash(s.File)))
	return err == nil && st.Mode().IsRegular()
}
