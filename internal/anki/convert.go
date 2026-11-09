package anki

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

// Card is one imported question and answer, ready to become a "Q :: R" line.
type Card struct {
	Deck   string
	Q, A   string
	CardID string // set when the note came from a Road to Knowledge export
	Media  []string
}

// Stats says what an import did, so the CLI can report it in one line.
type Stats struct {
	Notes, Cards, Cloze, Known, ToSubjects, ToInbox int
	MediaCopied, MediaSkipped, Sounds, Unsupported  int
}

var (
	blockTag  = regexp.MustCompile(`(?i)<\s*(br|/?div|/?p|/?li|/?h[1-6]|/?tr)\b[^>]*>`)
	imgTag    = regexp.MustCompile(`(?i)<img\b[^>]*?\bsrc\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))[^>]*>`)
	anyTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	sound     = regexp.MustCompile(`\[sound:([^\]]+)\]`)
	space     = regexp.MustCompile(`[ \t\x{a0}]+`)
	clozeMark = regexp.MustCompile(`(?s)\{\{c(\d+)::(.*?)(?:::(.*?))?\}\}`)
	unsafe    = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// Text turns an Anki field into one line of plain text. Images and sounds are returned apart:
// a card line is shown escaped in the app, so they go on their own line below it.
func Text(field string) (string, []string, int) {
	var media []string
	s := imgTag.ReplaceAllStringFunc(field, func(m string) string {
		sub := imgTag.FindStringSubmatch(m)
		for _, src := range sub[2:] {
			if src != "" {
				media = append(media, html.UnescapeString(src))
				break
			}
		}
		return " "
	})
	sounds := 0
	s = sound.ReplaceAllStringFunc(s, func(m string) string {
		media = append(media, sound.FindStringSubmatch(m)[1])
		sounds++
		return " "
	})
	s = blockTag.ReplaceAllString(s, "\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(space.ReplaceAllString(line, " ")); line != "" {
			parts = append(parts, line)
		}
	}
	// " :: " separates question and answer in the repo, so it cannot survive inside either side.
	return strings.ReplaceAll(strings.Join(parts, " / "), " :: ", " : "), media, sounds
}

// Cards expands a note: Road to Knowledge notes keep their card id, cloze notes give one card
// per deletion, any other note type reads its first field as the question and the second as the answer.
func Cards(n RawNote, st *Stats) []Card {
	field := func(name string, fallback int) string {
		for i, f := range n.FieldNames {
			if f == name && i < len(n.Fields) {
				return n.Fields[i]
			}
		}
		if fallback >= 0 && fallback < len(n.Fields) {
			return n.Fields[fallback]
		}
		return ""
	}
	if id := strings.TrimSpace(anyTag.ReplaceAllString(field("RtkID", -1), "")); id != "" {
		q, mq, sq := Text(field("Front", 0))
		a, ma, sa := Text(field("Back", 1))
		st.Sounds += sq + sa
		return []Card{{Deck: n.Deck, Q: q, A: a, CardID: id, Media: append(mq, ma...)}}
	}
	if len(n.Fields) > 0 && clozeMark.MatchString(n.Fields[0]) {
		st.Cloze++
		return clozeCards(n, st)
	}
	if len(n.Fields) < 2 {
		st.Unsupported++
		return nil
	}
	q, mq, sq := Text(n.Fields[0])
	a, ma, sa := Text(n.Fields[1])
	st.Sounds += sq + sa
	if q == "" && len(mq) > 0 {
		q = "Image"
	}
	if q == "" || a == "" {
		st.Unsupported++
		return nil
	}
	return []Card{{Deck: n.Deck, Q: q, A: a, Media: append(mq, ma...)}}
}

func clozeCards(n RawNote, st *Stats) []Card {
	src := n.Fields[0]
	seen := map[int]bool{}
	var nums []int
	for _, m := range clozeMark.FindAllStringSubmatch(src, -1) {
		k, _ := strconv.Atoi(m[1])
		if !seen[k] {
			seen[k] = true
			nums = append(nums, k)
		}
	}
	sort.Ints(nums)
	var out []Card
	for _, k := range nums {
		var answers []string
		masked := clozeMark.ReplaceAllStringFunc(src, func(m string) string {
			sub := clozeMark.FindStringSubmatch(m)
			if n, _ := strconv.Atoi(sub[1]); n != k {
				return sub[2]
			}
			answers = append(answers, sub[2])
			if sub[3] != "" {
				return "[" + sub[3] + "]"
			}
			return "[…]"
		})
		q, mq, sq := Text(masked)
		a, _, _ := Text(strings.Join(answers, ", "))
		st.Sounds += sq
		if q != "" && a != "" {
			out = append(out, Card{Deck: n.Deck, Q: q, A: a, Media: mq})
		}
	}
	return out
}

// Plan turns a collection into file changes. Cards the repo already has are skipped. With
// toSubjects, cards exported from the repo go back into their subject file (for a proposal
// branch); every other card goes to inbox/anki-<deck>.md for triage.
func Plan(u *kb.Universe, col *Collection, toSubjects bool, now time.Time) ([]gitx.FileChange, Stats) {
	var st Stats
	known := map[string]bool{}
	questions := map[string]bool{}
	for _, c := range u.Cards() {
		known[c.ID] = true
		questions[norm(c.Question)] = true
	}
	mediaPath := map[string]string{}
	var assets []gitx.FileChange
	assetFor := func(name string) (string, bool) {
		if p, ok := mediaPath[name]; ok {
			return p, p != ""
		}
		data, ok := col.Media[name]
		base := unsafe.ReplaceAllString(filepath.Base(name), "_")
		sum := sha1.Sum(data)
		p := "assets/anki/" + hex.EncodeToString(sum[:4]) + "-" + base
		if _, err := gitx.ValidAssetPath(p); !ok || err != nil {
			mediaPath[name] = ""
			st.MediaSkipped++
			return "", false
		}
		mediaPath[name] = p
		assets = append(assets, gitx.FileChange{Path: p, Data: data})
		st.MediaCopied++
		return p, true
	}
	line := func(c Card) string {
		var b strings.Builder
		fmt.Fprintf(&b, "- %s :: %s\n", c.Q, c.A)
		for _, m := range c.Media {
			if p, ok := assetFor(m); ok {
				if isImage(p) {
					fmt.Fprintf(&b, "  ![](%s)\n", p)
				} else {
					fmt.Fprintf(&b, "  [%s](%s)\n", filepath.Base(m), p)
				}
			}
		}
		return b.String()
	}

	bySubject := map[string][]string{}
	byDeck := map[string][]string{}
	for _, n := range col.Notes {
		st.Notes++
		for _, c := range Cards(n, &st) {
			st.Cards++
			if (c.CardID != "" && known[c.CardID]) || questions[norm(c.Q)] {
				st.Known++
				continue
			}
			questions[norm(c.Q)] = true
			if star := starOf(u, c.CardID); toSubjects && star != nil {
				bySubject[star.ID] = append(bySubject[star.ID], line(c))
				st.ToSubjects++
				continue
			}
			deck := c.Deck
			if deck == "" {
				deck = "Anki"
			}
			byDeck[deck] = append(byDeck[deck], line(c))
			st.ToInbox++
		}
	}

	var files []gitx.FileChange
	for _, id := range sortedKeys(bySubject) {
		s := u.Star(id)
		rel, _ := filepath.Rel(u.Root, s.Path)
		body, _ := os.ReadFile(s.Path)
		files = append(files, gitx.FileChange{Path: filepath.ToSlash(rel), Content: strings.TrimRight(string(body), "\n") + "\n\n" + strings.Join(bySubject[id], "")})
	}
	for _, deck := range sortedKeys(byDeck) {
		var b strings.Builder
		fmt.Fprintf(&b, "---\ncaptured: %s\nsource: anki\ndeck: %q\n---\n", now.Format(time.RFC3339), deck)
		fmt.Fprintf(&b, "Cartes importées du deck Anki « %s ». Range-les dans les sujets qui leur correspondent.\n\n", deck)
		b.WriteString(strings.Join(byDeck[deck], ""))
		files = append(files, gitx.FileChange{Path: "inbox/anki-" + slug(deck) + ".md", Content: b.String()})
	}
	return append(files, assets...), st
}

func starOf(u *kb.Universe, cardID string) *kb.Star {
	i := strings.LastIndex(cardID, ":")
	if i <= 0 {
		return nil
	}
	return u.Star(cardID[:i])
}

func norm(q string) string { return strings.ToLower(strings.Join(strings.Fields(q), " ")) }

func slug(deck string) string {
	if s := kb.Slug(strings.ReplaceAll(deck, "::", " ")); s != "" {
		return s
	}
	return "deck"
}

func isImage(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
