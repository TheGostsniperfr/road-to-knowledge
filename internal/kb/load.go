package kb

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const GalaxiesDir = "galaxies"

type galaxyMeta struct {
	Name        string     `yaml:"name"`
	Hue         *float64   `yaml:"hue"`
	Shape       string     `yaml:"shape"`
	ReviewAfter int        `yaml:"review_after_days"`
	Discovery   *Discovery `yaml:"discovery"`
	Sources     []Feed     `yaml:"sources"`
}

type clusterMeta struct {
	Name    string `yaml:"name"`
	Sources []Feed `yaml:"sources"`
}

type starMeta struct {
	Name     string   `yaml:"name"`
	Level    int      `yaml:"level"`
	State    State    `yaml:"state"`
	Links    []string `yaml:"links"`
	Evidence []string `yaml:"evidence"`
	Tags     []string `yaml:"tags"`
	Reviewed string   `yaml:"reviewed"`
	Source   *Source  `yaml:"source"`
}

// Load reads every node under root/galaxies. It returns problems instead of failing on a single bad file.
func Load(root string) (*Universe, []Problem, error) {
	u := &Universe{Root: root, galaxies: map[string]*Galaxy{}, clusters: map[string]*Cluster{}, stars: map[string]*Star{}}
	var problems []Problem
	report := func(path, format string, args ...any) {
		rel, _ := filepath.Rel(root, path)
		problems = append(problems, Problem{Path: filepath.ToSlash(rel), Message: fmt.Sprintf(format, args...)})
	}

	gdirs, err := subdirs(filepath.Join(root, GalaxiesDir))
	if err != nil {
		return nil, nil, fmt.Errorf("load: read galaxies: %w", err)
	}
	for _, gdir := range gdirs {
		gid := filepath.Base(gdir)
		g := &Galaxy{ID: gid, Name: gid, Hue: hueOf(gid), Shape: "spiral", ReviewAfter: 90, Discovery: Discovery{Near: 60, TwoHops: 30, OffPiste: 10}}
		g.Path = filepath.Join(gdir, "_galaxy.md")
		var gm galaxyMeta
		if body, ok := readNode(g.Path, &gm, report, false); ok {
			g.Body = body
			if gm.Name != "" {
				g.Name = gm.Name
			}
			if gm.Hue != nil {
				g.Hue = *gm.Hue
			}
			if gm.Shape != "" {
				if !Shapes[gm.Shape] {
					report(g.Path, "unknown shape %q", gm.Shape)
				} else {
					g.Shape = gm.Shape
				}
			}
			if gm.ReviewAfter > 0 {
				g.ReviewAfter = gm.ReviewAfter
			}
			if gm.Discovery != nil {
				if gm.Discovery.Valid() {
					g.Discovery = *gm.Discovery
				} else {
					report(g.Path, "discovery shares must be positive and sum to 100, got %d + %d + %d", gm.Discovery.Near, gm.Discovery.TwoHops, gm.Discovery.OffPiste)
				}
			}
			g.Sources = validSources(g.Path, gm.Sources, report)
		}
		u.galaxies[gid] = g
		u.Galaxies = append(u.Galaxies, g)

		cdirs, err := subdirs(gdir)
		if err != nil {
			return nil, nil, fmt.Errorf("load: read %s: %w", gdir, err)
		}
		if loose, _ := filepath.Glob(filepath.Join(gdir, "[^_]*.md")); len(loose) > 0 {
			for _, p := range loose {
				report(p, "a subject must live in a sub-theme folder: galaxies/<theme>/<sub-theme>/<subject>.md")
			}
		}
		for _, cdir := range cdirs {
			cid := gid + "/" + filepath.Base(cdir)
			c := &Cluster{ID: cid, Name: filepath.Base(cdir), Galaxy: g, Path: filepath.Join(cdir, "_cluster.md")}
			var cm clusterMeta
			if body, ok := readNode(c.Path, &cm, report, false); ok {
				c.Body = body
				if cm.Name != "" {
					c.Name = cm.Name
				}
				c.Sources = validSources(c.Path, cm.Sources, report)
			}
			u.clusters[cid] = c
			g.Clusters = append(g.Clusters, c)

			files, _ := filepath.Glob(filepath.Join(cdir, "*.md"))
			sort.Strings(files)
			for _, f := range files {
				if strings.HasPrefix(filepath.Base(f), "_") {
					continue
				}
				sid := cid + "/" + strings.TrimSuffix(filepath.Base(f), ".md")
				var sm starMeta
				body, ok := readNode(f, &sm, report, true)
				if !ok {
					continue
				}
				s := &Star{ID: sid, Name: sm.Name, Level: sm.Level, State: sm.State, Links: sm.Links, Evidence: sm.Evidence, Tags: sm.Tags, Body: body, Path: f, Cluster: c}
				if s.Name == "" {
					s.Name = strings.TrimSuffix(filepath.Base(f), ".md")
				}
				if sm.Source != nil {
					if bad := sm.Source.Problems(); len(bad) > 0 {
						report(f, "%s", strings.Join(bad, "; "))
					} else {
						s.Source = sm.Source
					}
				}
				if s.Level < 0 || s.Level > 5 {
					report(f, "level must be between 0 and 5, got %d", s.Level)
					s.Level = max(0, min(5, s.Level))
				}
				if s.State == "" {
					s.State = defaultState(s.Level)
				} else if !s.State.Valid() {
					report(f, "unknown state %q", s.State)
					s.State = defaultState(s.Level)
				}
				if sm.Reviewed != "" {
					t, err := time.Parse("2006-01-02", sm.Reviewed)
					if err != nil {
						report(f, "reviewed must be YYYY-MM-DD, got %q", sm.Reviewed)
					}
					s.Reviewed = t
				}
				s.Summary = summarize(body)
				s.Planets = extractPlanets(sid, body)
				s.Cards = extractCards(sid, body)
				seen := map[string]bool{}
				for _, card := range s.Cards {
					if seen[card.ID] {
						report(f, "question %q appears twice; only the first card is kept for review and export", card.Question)
					}
					seen[card.ID] = true
				}
				u.stars[sid] = s
				c.Stars = append(c.Stars, s)
			}
		}
	}

	resolveLinks(u, report)
	applyLayout(u, report)
	loadPaths(u, report)
	return u, problems, nil
}

func applyLayout(u *Universe, report func(string, string, ...any)) {
	path := filepath.Join(u.Root, LayoutFile)
	l, err := readLayout(u.Root)
	if err != nil {
		report(path, "%v", err)
		return
	}
	for _, id := range sortedPinIDs(l.Galaxies) {
		pin := l.Galaxies[id]
		g := u.galaxies[id]
		switch {
		case g == nil:
			report(path, "pin for unknown galaxy %q", id)
		case !pin.Valid():
			report(path, "pin for %q must be finite and within ±%g", id, MaxCoord)
		default:
			g.Pin = &pin
		}
	}
}

func sortedPinIDs(m map[string]Pin) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// resolveLinks accepts either a full id or a bare subject slug, and makes wormholes symmetric.
func resolveLinks(u *Universe, report func(string, string, ...any)) {
	bySlug := map[string][]*Star{}
	for _, s := range u.Stars() {
		slug := s.ID[strings.LastIndex(s.ID, "/")+1:]
		bySlug[slug] = append(bySlug[slug], s)
	}
	resolved := map[string]map[string]bool{}
	addLink := func(a, b string) {
		if resolved[a] == nil {
			resolved[a] = map[string]bool{}
		}
		resolved[a][b] = true
	}
	for _, s := range u.Stars() {
		for _, l := range s.Links {
			target := u.stars[l]
			if target == nil {
				if c := bySlug[l]; len(c) == 1 {
					target = c[0]
				} else if len(c) > 1 {
					report(s.Path, "link %q is ambiguous, use the full id", l)
					continue
				}
			}
			if target == nil {
				report(s.Path, "link %q points to no subject", l)
				continue
			}
			if target != s {
				addLink(s.ID, target.ID)
				addLink(target.ID, s.ID)
			}
		}
	}
	for _, s := range u.Stars() {
		s.Links = s.Links[:0]
		for id := range resolved[s.ID] {
			s.Links = append(s.Links, id)
		}
		sort.Strings(s.Links)
	}
}

func readNode(path string, meta any, report func(string, string, ...any), required bool) (string, bool) {
	src, err := os.ReadFile(path)
	if err != nil {
		if required || !os.IsNotExist(err) {
			report(path, "cannot read: %v", err)
		}
		return "", false
	}
	front, body, err := splitFrontmatter(src)
	if err != nil {
		if err == errNoFrontmatter {
			return string(body), true
		}
		report(path, "%v", err)
		return "", false
	}
	if err := yaml.Unmarshal(front, meta); err != nil {
		report(path, "invalid frontmatter: %v", err)
		return "", false
	}
	return string(body), true
}

func subdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}

func defaultState(level int) State {
	switch {
	case level == 0:
		return StateExplore
	case level >= 3:
		return StateAcquired
	default:
		return StateProgress
	}
}

func hueOf(id string) float64 {
	h := fnv.New32a()
	h.Write([]byte(id))
	return float64(h.Sum32() % 360)
}
