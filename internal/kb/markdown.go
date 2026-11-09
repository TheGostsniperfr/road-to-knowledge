package kb

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

var errNoFrontmatter = errors.New("missing YAML frontmatter")

// splitFrontmatter returns the YAML between the leading "---" fences and the body after them.
func splitFrontmatter(src []byte) (front, body []byte, err error) {
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
		return nil, src, errNoFrontmatter
	}
	rest := src[bytes.IndexByte(src, '\n')+1:]
	for i := 0; i < len(rest); {
		end := bytes.IndexByte(rest[i:], '\n')
		line := rest[i:]
		if end >= 0 {
			line = rest[i : i+end]
		}
		if string(bytes.TrimRight(line, "\r")) == "---" {
			if end < 0 {
				return rest[:i], nil, nil
			}
			return rest[:i], rest[i+end+1:], nil
		}
		if end < 0 {
			break
		}
		i += end + 1
	}
	return nil, src, errors.New("unterminated YAML frontmatter")
}

var (
	cardLine = regexp.MustCompile(`^\s*(?:[-*]\s+)?(.+?)\s+::\s+(.+?)\s*$`)
	fence    = regexp.MustCompile("^\\s*(```|~~~)")
	nonSlug  = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// extractCards finds inline "question :: answer" lines outside code fences.
func extractCards(starID, body string) []Card {
	var cards []Card
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := cardLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		sum := sha1.Sum([]byte(strings.TrimSpace(m[1])))
		cards = append(cards, Card{ID: starID + ":" + hex.EncodeToString(sum[:4]), StarID: starID, Question: m[1], Answer: m[2]})
	}
	return cards
}

// extractPlanets turns each "## heading" section into a planet: the notions orbiting the subject.
func extractPlanets(starID, body string) []Planet {
	var planets []Planet
	inFence := false
	var cur *Planet
	var buf []string
	flush := func() {
		if cur != nil {
			cur.Body = strings.TrimSpace(strings.Join(buf, "\n"))
			planets = append(planets, *cur)
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			flush()
			name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			cur, buf = &Planet{ID: starID + "#" + Slug(name), Name: name}, nil
			continue
		}
		if cur != nil {
			buf = append(buf, line)
		}
	}
	flush()
	return planets
}

// summarize returns the first paragraph of the body, used for lists and token-cheap AI answers.
func summarize(body string) string {
	var para []string
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if fence.MatchString(t) {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "![") || cardLine.MatchString(t) {
			if len(para) > 0 {
				break
			}
			continue
		}
		if t == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, t)
	}
	s := strings.Join(para, " ")
	if r := []rune(s); len(r) > 280 {
		s = string(r[:277]) + "..."
	}
	return s
}

func Slug(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}
