package app

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

type Bucket string

const (
	BucketNear     Bucket = "near"
	BucketTwoHops  Bucket = "two_hops"
	BucketOffPiste Bucket = "off_piste"
)

const (
	DefaultFrontierLimit = 9
	MaxFrontierLimit     = 50
)

type Suggestion struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Galaxy  string   `json:"galaxy"`
	Cluster string   `json:"cluster"`
	Level   int      `json:"level"`
	State   kb.State `json:"state"`
	Bucket  Bucket   `json:"bucket"`
	Reason  string   `json:"reason"`
	score   float64
}

type FrontierView struct {
	Scope       string       `json:"scope"`
	Budget      kb.Discovery `json:"budget"`
	Suggestions []Suggestion `json:"suggestions"`
}

// known subjects anchor the frontier; a level-0 subject marked acquired is not something to build on.
func isKnown(s *kb.Star) bool { return s.Level >= 3 || (s.State == kb.StateAcquired && s.Level > 0) }

// A mastered subject left in idea or explore by hand is never suggested.
func isCandidate(s *kb.Star) bool {
	return (s.State == kb.StateIdea || s.State == kb.StateExplore) && s.Level < 3
}

// Frontier ranks what to learn next around a scope (empty for the universe, a galaxy id or a cluster id).
// Near and two-hop suggestions come from the scope. Off-piste ones are subjects with no known neighbour:
// those of the scope itself, and those one level above it, so a cluster scope can point to a
// neighbouring cluster and a galaxy scope to another galaxy.
func Frontier(u *kb.Universe, scope string, limit int) (FrontierView, error) {
	if limit <= 0 {
		limit = DefaultFrontierLimit
	}
	limit = min(limit, MaxFrontierLimit)

	inScope, inParent, budget, err := scopeFilters(u, scope)
	if err != nil {
		return FrontierView{}, err
	}

	var near, twoHops, offPiste []Suggestion
	for _, s := range u.Stars() {
		if !isCandidate(s) || !inParent(s) {
			continue
		}
		sg := Suggestion{ID: s.ID, Name: s.Name, Galaxy: s.Cluster.Galaxy.Name, Cluster: s.Cluster.Name, Level: s.Level, State: s.State}
		if inScope(s) {
			if reason, score, ok := nearReason(u, s); ok {
				sg.Bucket, sg.Reason, sg.score = BucketNear, reason, score
				near = append(near, sg)
				continue
			}
			if reason, score, ok := twoHopReason(u, s); ok {
				sg.Bucket, sg.Reason, sg.score = BucketTwoHops, reason, score
				twoHops = append(twoHops, sg)
				continue
			}
		}
		cov := coverage(s.Cluster)
		sg.Bucket, sg.score = BucketOffPiste, 1-cov
		if inScope(s) && scope != "" {
			sg.Reason = fmt.Sprintf("terrain neuf dans %s, rien d'acquis autour", s.Cluster.Name)
		} else {
			sg.Reason = fmt.Sprintf("%s (%s), exploré à %d %%", s.Cluster.Name, s.Cluster.Galaxy.Name, int(math.Round(cov*100)))
		}
		offPiste = append(offPiste, sg)
	}

	pools := [][]Suggestion{near, twoHops, offPiste}
	for _, p := range pools {
		sort.SliceStable(p, func(i, j int) bool {
			if p[i].score != p[j].score {
				return p[i].score > p[j].score
			}
			return p[i].ID < p[j].ID
		})
		interleave(p, depthOf(scope))
	}
	quotas := split(limit, []int{budget.Near, budget.TwoHops, budget.OffPiste})
	out := []Suggestion{}
	taken := make([]int, len(pools))
	for i, p := range pools {
		taken[i] = min(quotas[i], len(p))
	}
	// A bucket with too few candidates hands its slots to the others, in budget order.
	for spare := limit - sum(taken); spare > 0; {
		moved := false
		for i, p := range pools {
			if spare > 0 && taken[i] < len(p) {
				taken[i]++
				spare--
				moved = true
			}
		}
		if !moved {
			break
		}
	}
	for i, p := range pools {
		out = append(out, p[:taken[i]]...)
	}
	return FrontierView{Scope: scope, Budget: budget, Suggestions: out}, nil
}

func scopeFilters(u *kb.Universe, scope string) (inScope, inParent func(*kb.Star) bool, budget kb.Discovery, err error) {
	all := func(*kb.Star) bool { return true }
	switch {
	case scope == "":
		return all, all, averageBudget(u.Galaxies), nil
	case u.Galaxy(scope) != nil:
		g := u.Galaxy(scope)
		return func(s *kb.Star) bool { return s.Cluster.Galaxy == g }, all, g.Discovery, nil
	case u.Cluster(scope) != nil:
		c := u.Cluster(scope)
		return func(s *kb.Star) bool { return s.Cluster == c },
			func(s *kb.Star) bool { return s.Cluster.Galaxy == c.Galaxy },
			c.Galaxy.Discovery, nil
	}
	return nil, nil, kb.Discovery{}, fmt.Errorf("frontier: unknown scope %q: %w", scope, ErrNotFound)
}

func depthOf(scope string) int {
	if scope == "" {
		return 0
	}
	return strings.Count(scope, "/") + 1
}

// interleave takes the best of each child of the scope in turn (galaxies for the universe, sub-themes
// for a galaxy), so one dense theme cannot fill every slot. Ranks inside a child are kept.
func interleave(p []Suggestion, depth int) {
	if depth >= 2 {
		return
	}
	child := func(id string) string { return strings.Join(strings.SplitN(id, "/", 3)[:depth+1], "/") }
	groups := map[string][]Suggestion{}
	var order []string
	for _, s := range p {
		k := child(s.ID)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], s)
	}
	out := p[:0:0]
	for len(out) < len(p) {
		for _, k := range order {
			if g := groups[k]; len(g) > 0 {
				out = append(out, g[0])
				groups[k] = g[1:]
			}
		}
	}
	copy(p, out)
}

func averageBudget(gs []*kb.Galaxy) kb.Discovery {
	if len(gs) == 0 {
		return kb.Discovery{Near: 60, TwoHops: 30, OffPiste: 10}
	}
	var d kb.Discovery
	for _, g := range gs {
		d.Near += g.Discovery.Near
		d.TwoHops += g.Discovery.TwoHops
		d.OffPiste += g.Discovery.OffPiste
		d.News = d.News || g.Discovery.News
	}
	n := len(gs)
	d.Near, d.TwoHops, d.OffPiste = d.Near/n, d.TwoHops/n, d.OffPiste/n
	return d
}

// split shares n slots by weight with the largest remainder method, so 9 slots at 60/30/10 give 5/3/1.
func split(n int, weights []int) []int {
	total := 0
	for _, w := range weights {
		total += max(w, 0)
	}
	out := make([]int, len(weights))
	if total <= 0 {
		out[0] = n
		return out
	}
	type rem struct {
		i int
		r float64
	}
	var rs []rem
	used := 0
	for i, w := range weights {
		w = max(w, 0)
		exact := float64(n) * float64(w) / float64(total)
		out[i] = int(exact)
		used += out[i]
		rs = append(rs, rem{i, exact - float64(out[i])})
	}
	sort.SliceStable(rs, func(a, b int) bool { return rs[a].r > rs[b].r })
	for k := 0; used < n; k++ {
		out[rs[k%len(rs)].i]++
		used++
	}
	return out
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

func knownLinks(u *kb.Universe, s *kb.Star) []*kb.Star {
	var out []*kb.Star
	for _, id := range s.Links {
		if l := u.Star(id); l != nil && isKnown(l) {
			out = append(out, l)
		}
	}
	return out
}

func strongest(stars []*kb.Star) *kb.Star {
	sort.SliceStable(stars, func(i, j int) bool {
		if stars[i].Level != stars[j].Level {
			return stars[i].Level > stars[j].Level
		}
		return stars[i].Name < stars[j].Name
	})
	return stars[0]
}

func nearReason(u *kb.Universe, s *kb.Star) (string, float64, bool) {
	links := knownLinks(u, s)
	var mates []*kb.Star
	for _, m := range s.Cluster.Stars {
		if m != s && isKnown(m) {
			mates = append(mates, m)
		}
	}
	if len(links) == 0 && len(mates) == 0 {
		return "", 0, false
	}
	score := float64(len(mates))
	for _, l := range links {
		score += 2 * float64(l.Level)
	}
	if len(links) > 0 {
		best := strongest(links)
		reason := fmt.Sprintf("lié à %s que tu maîtrises", best.Name)
		if len(links) > 1 {
			reason = fmt.Sprintf("lié à %s et %d autre%s que tu maîtrises", best.Name, len(links)-1, plural(len(links)-1))
		}
		return reason, score, true
	}
	if len(mates) == 1 {
		return fmt.Sprintf("dans %s, à côté de %s que tu maîtrises", s.Cluster.Name, mates[0].Name), score, true
	}
	return fmt.Sprintf("dans %s, à côté de %d sujets que tu maîtrises", s.Cluster.Name, len(mates)), score, true
}

func twoHopReason(u *kb.Universe, s *kb.Star) (string, float64, bool) {
	var bestKnown, bestVia *kb.Star
	for _, viaID := range s.Links {
		via := u.Star(viaID)
		if via == nil {
			continue
		}
		for _, k := range knownLinks(u, via) {
			if k == s {
				continue
			}
			if bestKnown == nil || k.Level > bestKnown.Level ||
				(k.Level == bestKnown.Level && (k.Name < bestKnown.Name || (k.Name == bestKnown.Name && via.Name < bestVia.Name))) {
				bestKnown, bestVia = k, via
			}
		}
	}
	if bestKnown == nil {
		return "", 0, false
	}
	return fmt.Sprintf("à deux pas de %s, via %s", bestKnown.Name, bestVia.Name), float64(bestKnown.Level), true
}

func coverage(c *kb.Cluster) float64 {
	if len(c.Stars) == 0 {
		return 0
	}
	n := 0
	for _, s := range c.Stars {
		if isKnown(s) {
			n++
		}
	}
	return float64(n) / float64(len(c.Stars))
}

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

func (s *Service) Frontier(scope string, limit int) (FrontierView, error) {
	u, _, _, _ := s.snapshot()
	return Frontier(u, strings.Trim(scope, "/"), limit)
}
