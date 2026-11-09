// Package anki moves review cards between a knowledge repository and Anki: .apkg export and
// import, and a push through the AnkiConnect add-on. The mapping is ADR-0010.
package anki

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

// ModelName is the note type every exported card uses. Its fourth field carries the card id,
// which is how an import or a push recognises a card that came from the repo.
const ModelName = "Road to Knowledge"

var Fields = []string{"Front", "Back", "Subject", "RtkID"}

var ErrScope = errors.New("unknown scope")

// Note is one exported card: one Anki note with one card.
type Note struct {
	CardID  string
	GUID    string
	Deck    string
	Front   string
	Back    string
	Subject string
	Tags    []string
}

// Collect lists the cards of a scope (universe "", galaxy, sub-theme or subject id) in repo order.
func Collect(u *kb.Universe, scope string) ([]Note, error) {
	var stars []*kb.Star
	switch {
	case scope == "":
		stars = u.Stars()
	case u.Galaxy(scope) != nil:
		for _, c := range u.Galaxy(scope).Clusters {
			stars = append(stars, c.Stars...)
		}
	case u.Cluster(scope) != nil:
		stars = u.Cluster(scope).Stars
	case u.Star(scope) != nil:
		stars = []*kb.Star{u.Star(scope)}
	default:
		return nil, fmt.Errorf("%w: %q", ErrScope, scope)
	}
	var out []Note
	for _, s := range stars {
		g, c := s.Cluster.Galaxy, s.Cluster
		tags := []string{"rtk", "rtk::" + tagPart(g.ID) + "::" + tagPart(lastPart(c.ID))}
		for _, t := range s.Tags {
			tags = append(tags, tagPart(t))
		}
		for _, card := range s.Cards {
			out = append(out, Note{
				CardID:  card.ID,
				GUID:    GUID(card.ID),
				Deck:    DeckName(g.Name, c.Name),
				Front:   card.Question,
				Back:    card.Answer,
				Subject: g.Name + " › " + c.Name + " › " + s.Name,
				Tags:    tags,
			})
		}
	}
	return out, nil
}

// DeckName nests decks as RtK::Galaxy::Sub-theme. "::" is Anki's separator, so it never appears inside a name.
func DeckName(galaxy, cluster string) string {
	clean := func(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "::", ":")) }
	return "RtK::" + clean(galaxy) + "::" + clean(cluster)
}

const base91 = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&()*+,-./:;<=>?@[]^_`{|}~"

// GUID derives Anki's note guid from the card id, in the same base91 form Anki uses for its own
// random guids. The same card always gets the same guid, so a re-import updates the note in Anki.
func GUID(cardID string) string {
	sum := sha1.Sum([]byte("rtk:" + cardID))
	v := binary.BigEndian.Uint64(sum[:8])
	var b []byte
	for v > 0 {
		b = append(b, base91[v%91])
		v /= 91
	}
	if len(b) == 0 {
		b = []byte{base91[0]}
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// stableID gives deterministic positive ids below 2^52, the range Anki uses for millisecond ids.
func stableID(kind, key string) int64 {
	sum := sha1.Sum([]byte(kind + ":" + key))
	id := int64(binary.BigEndian.Uint64(sum[:8]) & (1<<52 - 1))
	if id < 2 {
		id += 2
	}
	return id
}

func tagPart(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "::", "_")), "_")
}

func lastPart(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

func decksOf(notes []Note) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range notes {
		if !seen[n.Deck] {
			seen[n.Deck] = true
			out = append(out, n.Deck)
		}
	}
	sort.Strings(out)
	return out
}
