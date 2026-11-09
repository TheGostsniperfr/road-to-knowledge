package kb

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// SetFields rewrites only the given frontmatter keys, keeping key order, comments and the body untouched.
func SetFields(path string, fields map[string]any) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("set fields: %w", err)
	}
	front, body, err := splitFrontmatter(src)
	if err != nil && err != errNoFrontmatter {
		return fmt.Errorf("set fields %s: %w", path, err)
	}
	var doc yaml.Node
	if len(bytes.TrimSpace(front)) > 0 {
		if err := yaml.Unmarshal(front, &doc); err != nil {
			return fmt.Errorf("set fields %s: %w", path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	m := doc.Content[0]
	for _, key := range sortedKeys(fields) {
		var v yaml.Node
		if err := v.Encode(fields[key]); err != nil {
			return fmt.Errorf("set fields: encode %s: %w", key, err)
		}
		if v.Kind == yaml.MappingNode {
			v.Style = yaml.FlowStyle
		}
		set := false
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == key {
				m.Content[i+1] = &v
				set = true
				break
			}
		}
		if !set {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &v)
		}
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("set fields: encode: %w", err)
	}
	enc.Close()
	res := append([]byte("---\n"), out.Bytes()...)
	res = append(res, []byte("---\n")...)
	res = append(res, body...)
	return writeAtomic(path, res)
}

// writeAtomic uses a unique temp file in the same directory, so concurrent writers never share it
// and the rename stays on one filesystem.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ReadFile parses a Markdown file with optional frontmatter into meta and returns the body.
func ReadFile(path string, meta any) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	front, body, err := splitFrontmatter(src)
	if err == errNoFrontmatter {
		return string(body), nil
	}
	if err != nil {
		return "", err
	}
	if err := yaml.Unmarshal(front, meta); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return string(body), nil
}
