package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/review"
)

type PlanetRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type StarSummary struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Level   int         `json:"level"`
	State   kb.State    `json:"state"`
	Days    *int        `json:"days"`
	Stale   bool        `json:"stale"`
	Links   []string    `json:"links,omitempty"`
	Planets []PlanetRef `json:"planets,omitempty"`
	Cards   int         `json:"cards"`
	Due     int         `json:"due"`
}

type ClusterView struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Stars []StarSummary `json:"stars"`
}

type GalaxyView struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Hue         float64       `json:"hue"`
	Shape       string        `json:"shape"`
	ReviewAfter int           `json:"review_after_days"`
	Discovery   kb.Discovery  `json:"discovery"`
	Pin         *kb.Pin       `json:"pin,omitempty"`
	Clusters    []ClusterView `json:"clusters"`
}

type UniverseView struct {
	Version  int64        `json:"version"`
	Galaxies []GalaxyView `json:"galaxies"`
	Problems []kb.Problem `json:"problems"`
	Inbox    int          `json:"inbox"`
	// Redirects maps ids from before a restructure to today's, so old deep links still open.
	Redirects map[string]string `json:"redirects,omitempty"`
}

type LinkRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Galaxy string `json:"galaxy"`
}

type StarView struct {
	StarSummary
	Path     []string    `json:"path"`
	Summary  string      `json:"summary"`
	Body     string      `json:"body"`
	Evidence []string    `json:"evidence,omitempty"`
	Tags     []string    `json:"tags,omitempty"`
	LinkRefs []LinkRef   `json:"link_refs,omitempty"`
	File     string      `json:"file"`
	Source   *SourceView `json:"source,omitempty"`
	Steps    []StepRef   `json:"steps,omitempty"`
	// CardList lets a lesson page rate the cards it shows with their real ids.
	CardList []kb.Card `json:"card_list,omitempty"`
}

// SourceView adds what only the server can tell: whether the original file is on this machine.
type SourceView struct {
	kb.Source
	Available bool `json:"available"`
}

func (s *Service) days(st *kb.Star) *int {
	last := st.Reviewed
	ids := make([]string, len(st.Cards))
	for i, c := range st.Cards {
		ids[i] = c.ID
	}
	if r := s.book.LastReviewOf(ids); r.After(last) {
		last = r
	}
	if last.IsZero() {
		return nil
	}
	d := int(math.Floor(s.now().Sub(last).Hours() / 24))
	return &d
}

func (s *Service) summary(st *kb.Star) StarSummary {
	now := s.now()
	sum := StarSummary{ID: st.ID, Name: st.Name, Level: st.Level, State: st.State, Links: st.Links, Cards: len(st.Cards), Days: s.days(st)}
	for _, p := range st.Planets {
		sum.Planets = append(sum.Planets, PlanetRef{ID: p.ID, Name: p.Name})
	}
	for _, c := range st.Cards {
		if s.book.IsDue(c.ID, now) {
			sum.Due++
		}
	}
	// Erosion is only a hint: the level itself never drops.
	sum.Stale = st.Level > 0 && sum.Days != nil && *sum.Days > st.Cluster.Galaxy.ReviewAfter
	return sum
}

func (s *Service) starView(u *kb.Universe, st *kb.Star) StarView {
	rel, _ := filepath.Rel(s.root, st.Path)
	v := StarView{StarSummary: s.summary(st), Path: []string{st.Cluster.Galaxy.Name, st.Cluster.Name}, Summary: st.Summary, Body: st.Body, Evidence: st.Evidence, Tags: st.Tags, File: filepath.ToSlash(rel)}
	if st.Source != nil {
		v.Source = &SourceView{Source: *st.Source, Available: st.Source.Available(s.root)}
	}
	for _, l := range st.Links {
		if o := u.Star(l); o != nil {
			v.LinkRefs = append(v.LinkRefs, LinkRef{ID: o.ID, Name: o.Name, Galaxy: o.Cluster.Galaxy.Name})
		}
	}
	v.Steps = s.stepsCovering(u, st.ID)
	v.CardList = st.Cards
	return v
}

func (s *Service) Universe() UniverseView {
	u, _, problems, version := s.snapshot()
	out := UniverseView{Version: version, Problems: problems, Galaxies: []GalaxyView{}}
	if r := s.book.Redirects().All(); len(r) > 0 {
		out.Redirects = r
	}
	if out.Problems == nil {
		out.Problems = []kb.Problem{}
	}
	for _, g := range u.Galaxies {
		gv := GalaxyView{ID: g.ID, Name: g.Name, Hue: g.Hue, Shape: g.Shape, ReviewAfter: g.ReviewAfter, Discovery: g.Discovery, Pin: g.Pin, Clusters: []ClusterView{}}
		for _, c := range g.Clusters {
			cv := ClusterView{ID: c.ID, Name: c.Name, Stars: []StarSummary{}}
			for _, st := range c.Stars {
				cv.Stars = append(cv.Stars, s.summary(st))
			}
			gv.Clusters = append(gv.Clusters, cv)
		}
		out.Galaxies = append(out.Galaxies, gv)
	}
	entries, _ := os.ReadDir(filepath.Join(s.root, "inbox"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			out.Inbox++
		}
	}
	return out
}

func (s *Service) Star(id string) (StarView, error) {
	u, _, _, _ := s.snapshot()
	id = s.live(u, id)
	st := u.Star(id)
	if st == nil {
		return StarView{}, fmt.Errorf("star %q: %w", id, ErrNotFound)
	}
	return s.starView(u, st), nil
}

// --- token-cheap views for agents ---

type ProfileTheme struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Subjects int     `json:"subjects"`
	AvgLevel float64 `json:"avg_level"`
	Fog      int     `json:"fog"`
}

type Profile struct {
	Themes     []ProfileTheme `json:"themes"`
	Strongest  []string       `json:"strongest"`
	InProgress []string       `json:"in_progress"`
	Frontier   []string       `json:"frontier"`
	Stale      int            `json:"stale"`
	DueCards   int            `json:"due_cards"`
}

func (s *Service) Profile() Profile {
	u, _, _, _ := s.snapshot()
	p := Profile{Themes: []ProfileTheme{}}
	var all []StarSummary
	for _, g := range u.Galaxies {
		t := ProfileTheme{ID: g.ID, Name: g.Name}
		known, sum := 0, 0
		for _, c := range g.Clusters {
			for _, st := range c.Stars {
				ss := s.summary(st)
				all = append(all, ss)
				t.Subjects++
				if st.Level == 0 {
					t.Fog++
				} else {
					known++
					sum += st.Level
				}
				if ss.Stale {
					p.Stale++
				}
				p.DueCards += ss.Due
				switch st.State {
				case kb.StateProgress:
					p.InProgress = append(p.InProgress, st.ID)
				case kb.StateExplore, kb.StateIdea:
					p.Frontier = append(p.Frontier, st.ID)
				}
			}
		}
		if known > 0 {
			t.AvgLevel = math.Round(float64(sum)/float64(known)*10) / 10
		}
		p.Themes = append(p.Themes, t)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Level > all[j].Level })
	for i := 0; i < len(all) && i < 8 && all[i].Level >= 4; i++ {
		p.Strongest = append(p.Strongest, all[i].ID)
	}
	p.InProgress = capList(p.InProgress, 12)
	p.Frontier = capList(p.Frontier, 12)
	return p
}

type SubgraphNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Kind     kb.Kind        `json:"kind"`
	Level    *int           `json:"level,omitempty"`
	State    kb.State       `json:"state,omitempty"`
	Children []SubgraphNode `json:"children,omitempty"`
}

// Subgraph returns a node and its descendants down to depth, names and levels only.
func (s *Service) Subgraph(id string, depth int) (SubgraphNode, error) {
	u, _, _, _ := s.snapshot()
	id = s.live(u, id)
	depth = max(0, min(depth, 3))
	star := func(st *kb.Star) SubgraphNode {
		l := st.Level
		return SubgraphNode{ID: st.ID, Name: st.Name, Kind: kb.KindStar, Level: &l, State: st.State}
	}
	cluster := func(c *kb.Cluster, d int) SubgraphNode {
		n := SubgraphNode{ID: c.ID, Name: c.Name, Kind: kb.KindCluster}
		if d > 0 {
			for _, st := range c.Stars {
				n.Children = append(n.Children, star(st))
			}
		}
		return n
	}
	galaxy := func(g *kb.Galaxy, d int) SubgraphNode {
		n := SubgraphNode{ID: g.ID, Name: g.Name, Kind: kb.KindGalaxy}
		if d > 0 {
			for _, c := range g.Clusters {
				n.Children = append(n.Children, cluster(c, d-1))
			}
		}
		return n
	}
	switch {
	case id == "" || id == "/":
		root := SubgraphNode{ID: "", Name: "Univers"}
		for _, g := range u.Galaxies {
			root.Children = append(root.Children, galaxy(g, depth-1))
		}
		return root, nil
	case u.Galaxy(id) != nil:
		return galaxy(u.Galaxy(id), depth), nil
	case u.Cluster(id) != nil:
		return cluster(u.Cluster(id), depth), nil
	case u.Star(id) != nil:
		return star(u.Star(id)), nil
	}
	return SubgraphNode{}, fmt.Errorf("node %q: %w", id, ErrNotFound)
}

type NodeBrief struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Level     int      `json:"level"`
	State     kb.State `json:"state"`
	Days      *int     `json:"last_review_days"`
	Summary   string   `json:"summary"`
	Planets   []string `json:"planets,omitempty"`
	Wormholes []string `json:"wormholes,omitempty"`
	Evidence  []string `json:"evidence,omitempty"`
	Source    string   `json:"source,omitempty"`
	Cards     int      `json:"cards"`
	More      string   `json:"more,omitempty"`
}

// Node is the brief an agent gets first; the full text costs a separate fragments() call.
func (s *Service) Node(id string) (NodeBrief, error) {
	u, _, _, _ := s.snapshot()
	id = s.live(u, id)
	st := u.Star(id)
	if st == nil {
		return NodeBrief{}, fmt.Errorf("star %q: %w", id, ErrNotFound)
	}
	sum := s.summary(st)
	b := NodeBrief{ID: st.ID, Path: st.Cluster.Galaxy.Name + " / " + st.Cluster.Name + " / " + st.Name, Level: st.Level, State: st.State, Days: sum.Days, Summary: st.Summary, Wormholes: st.Links, Evidence: st.Evidence, Cards: len(st.Cards)}
	if src := st.Source; src != nil {
		b.Source = src.Title
		if src.Author != "" {
			b.Source += ", " + src.Author
		}
	}
	for _, p := range st.Planets {
		b.Planets = append(b.Planets, p.Name)
	}
	if len(strings.TrimSpace(st.Body)) > len(st.Summary)+20 {
		b.More = fmt.Sprintf("fragments(%q) for the full fiche", st.ID)
	}
	return b, nil
}

// Fragments returns the fiche body, or one section of it when section names a planet.
func (s *Service) Fragments(id, section string) (string, error) {
	u, _, _, _ := s.snapshot()
	id = s.live(u, id)
	st := u.Star(id)
	if st == nil {
		return "", fmt.Errorf("star %q: %w", id, ErrNotFound)
	}
	if section == "" {
		return strings.TrimSpace(st.Body), nil
	}
	for _, p := range st.Planets {
		if strings.EqualFold(p.Name, section) || p.ID == section {
			return "## " + p.Name + "\n" + p.Body, nil
		}
	}
	names := make([]string, len(st.Planets))
	for i, p := range st.Planets {
		names[i] = p.Name
	}
	return "", fmt.Errorf("section %q in %q: %w; sections: %s", section, id, ErrNotFound, strings.Join(names, ", "))
}

// --- review ---

type DueCard struct {
	ID       string            `json:"id"`
	StarID   string            `json:"star"`
	StarName string            `json:"star_name"`
	Question string            `json:"q"`
	Answer   string            `json:"a"`
	New      bool              `json:"new"`
	Next     map[string]string `json:"next"`
}

const maxNewPerSession = 20

// Due lists cards to review under scope (a galaxy, cluster or star id, or everything), minus excluded children.
func (s *Service) Due(scope string, exclude []string) ([]DueCard, error) {
	u, _, _, _ := s.snapshot()
	now := s.now()
	var stars []*kb.Star
	in := func(id string) bool {
		for _, x := range exclude {
			if id == x || strings.HasPrefix(id, x+"/") {
				return false
			}
		}
		return scope == "" || id == scope || strings.HasPrefix(id, scope+"/")
	}
	if scope != "" && u.Galaxy(scope) == nil && u.Cluster(scope) == nil && u.Star(scope) == nil {
		return nil, fmt.Errorf("scope %q: %w", scope, ErrNotFound)
	}
	for _, st := range u.Stars() {
		if in(st.ID) {
			stars = append(stars, st)
		}
	}
	var due, fresh []DueCard
	for _, st := range stars {
		for _, c := range st.Cards {
			cs := s.book.State(c.ID, now)
			if cs.Due.After(now) {
				continue
			}
			dc := DueCard{ID: c.ID, StarID: st.ID, StarName: st.Name, Question: c.Question, Answer: c.Answer, New: cs.IsNew, Next: map[string]string{}}
			for r, d := range s.book.Preview(c.ID, now) {
				dc.Next[fmt.Sprint(int(r))] = human(d)
			}
			if cs.IsNew {
				fresh = append(fresh, dc)
			} else {
				due = append(due, dc)
			}
		}
	}
	out := append(due, capList(fresh, maxNewPerSession)...)
	if out == nil {
		out = []DueCard{}
	}
	return out, nil
}

func (s *Service) Review(ctx context.Context, cardID string, rating int) error {
	u, _, _, _ := s.snapshot()
	known := func(id string) bool {
		for _, c := range u.Cards() {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	// A page opened before a restructure still sends the old id; a live id is never redirected.
	if !known(cardID) {
		cardID = s.book.Redirects().Card(cardID)
	}
	if !known(cardID) {
		return fmt.Errorf("card %q: %w", cardID, ErrNotFound)
	}
	if !review.Rating(rating).Valid() {
		return fmt.Errorf("rating %d: %w", rating, ErrInvalid)
	}
	_, err := s.book.Record(cardID, review.Rating(rating), s.now())
	return err
}

// FlushReviews commits the review log once per session instead of once per card.
func (s *Service) FlushReviews(ctx context.Context, n int) {
	s.commit(ctx, fmt.Sprintf("chore(review): %d cartes révisées", n), s.book.Path())
}

func human(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d min", max(1, int(d.Minutes())))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d j", int(math.Round(d.Hours()/24)))
	default:
		return fmt.Sprintf("%d mois", int(math.Round(d.Hours()/24/30)))
	}
}

func capList[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

type InboxItem struct {
	Name     string `json:"name"`
	Text     string `json:"text"`
	Captured string `json:"captured"`
}

func inboxKey(name string) (string, int) {
	base := strings.TrimSuffix(name, ".md")
	if i := strings.LastIndex(base, "-"); i > 0 {
		if n, err := strconv.Atoi(base[i+1:]); err == nil && len(base[i+1:]) < 4 {
			return base[:i], n
		}
	}
	return base, 1
}

// Inbox lists captured notes, newest first, waiting for triage.
func (s *Service) Inbox() ([]InboxItem, error) {
	dir := filepath.Join(s.root, "inbox")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inbox: %w", err)
	}
	// "…150405-2.md" sorts before "…150405.md" by name; order by stamp, then by suffix.
	sort.SliceStable(entries, func(i, j int) bool {
		si, ni := inboxKey(entries[i].Name())
		sj, nj := inboxKey(entries[j].Name())
		if si != sj {
			return si < sj
		}
		return ni < nj
	})
	out := []InboxItem{}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		var meta struct {
			Captured string `yaml:"captured"`
		}
		body, err := kb.ReadFile(filepath.Join(dir, e.Name()), &meta)
		if err != nil {
			continue
		}
		out = append(out, InboxItem{Name: e.Name(), Text: strings.TrimSpace(body), Captured: meta.Captured})
	}
	return out, nil
}
