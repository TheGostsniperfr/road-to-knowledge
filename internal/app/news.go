package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/news"
)

// NewsDir holds the feed cache. It is git-ignored: items are rebuilt from the sources at any time.
const NewsDir = ".rtk/news"

type NewsItem struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Link      string       `json:"link"`
	Summary   string       `json:"summary,omitempty"`
	Source    string       `json:"source"`
	Scope     string       `json:"scope"`
	ScopeName string       `json:"scope_name"`
	Galaxy    string       `json:"galaxy"`
	Published time.Time    `json:"published"`
	Subjects  []news.Match `json:"subjects"`
}

type NewsSource struct {
	URL       string     `json:"url"`
	Name      string     `json:"name"`
	Scope     string     `json:"scope"`
	ScopeName string     `json:"scope_name"`
	LastOK    *time.Time `json:"last_ok,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type NewsView struct {
	Items   []NewsItem   `json:"items"`
	Sources []NewsSource `json:"sources"`
	// Hidden counts items left out because their theme only wants news about known subjects.
	Hidden int `json:"hidden"`
}

type NewsQuery struct {
	Scope string
	Since time.Time
	Limit int
	// All also returns items that mention no known subject, even in themes whose discovery
	// budget has news off.
	All bool
}

func (s *Service) news() (*news.Store, error) {
	s.newsOnce.Do(func() { s.newsStore, s.newsErr = news.Open(filepath.Join(s.root, NewsDir)) })
	return s.newsStore, s.newsErr
}

// Feeds lists the sources declared in the knowledge repo, theme feeds first. A URL listed in
// several places is fetched once and belongs to its first listing: the broader scope, since a
// theme comes before its sub-themes. Matching still shows its items under any scope whose
// subjects they mention.
func (s *Service) Feeds() []news.Feed {
	u, _, _, _ := s.snapshot()
	var out []news.Feed
	seen := map[string]bool{}
	add := func(src kb.Feed, scope string) {
		if seen[src.URL] {
			return
		}
		seen[src.URL] = true
		out = append(out, news.Feed{URL: src.URL, Name: sourceName(src), Scope: scope})
	}
	for _, g := range u.Galaxies {
		for _, src := range g.Sources {
			add(src, g.ID)
		}
		for _, c := range g.Clusters {
			for _, src := range c.Sources {
				add(src, c.ID)
			}
		}
	}
	return out
}

func sourceName(src kb.Feed) string {
	if src.Name != "" {
		return src.Name
	}
	return strings.TrimPrefix(strings.TrimPrefix(src.URL, "https://"), "http://")
}

// FetchNews runs one pass over the feeds. Concurrent calls wait for each other rather than
// fetching the same feeds twice.
func (s *Service) FetchNews(ctx context.Context) (news.Report, error) {
	st, err := s.news()
	if err != nil {
		return news.Report{}, err
	}
	s.newsMu.Lock()
	defer s.newsMu.Unlock()
	f := news.NewFetcher()
	f.Now = s.now
	rep, err := f.Run(ctx, st, s.Feeds())
	if err == nil && rep.NewItems > 0 {
		s.notify(NewsEvent)
	}
	return rep, err
}

// NewsEvent goes to subscribers instead of a version number when new articles arrive, so an
// open "Veille" tab refreshes without reloading the whole universe.
const NewsEvent int64 = -1

// RunNews fetches shortly after start, then every interval, until ctx ends.
func (s *Service) RunNews(ctx context.Context, every time.Duration) {
	if every <= 0 {
		return
	}
	pass := func() {
		if len(s.Feeds()) == 0 {
			return
		}
		rep, err := s.FetchNews(ctx)
		if err != nil {
			s.log.Error("news fetch failed", "err", err)
			return
		}
		s.log.Info("news fetched", "feeds", rep.Fetched, "not_modified", rep.NotMod, "failed", rep.Failed, "new", rep.NewItems)
		for _, e := range rep.Errors {
			s.log.Warn("news feed failed", "err", e)
		}
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}
	pass()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass()
		}
	}
}

// News returns cached items, newest first, each tagged with the subjects it mentions. Matching
// happens on read, so a subject added today also tags last week's items.
func (s *Service) News(q NewsQuery) (NewsView, error) {
	u, _, _, _ := s.snapshot()
	if q.Scope != "" && u.Galaxy(q.Scope) == nil && u.Cluster(q.Scope) == nil {
		return NewsView{}, fmt.Errorf("scope %q: %w", q.Scope, ErrNotFound)
	}
	if q.Limit <= 0 {
		q.Limit = 30
	}
	q.Limit = min(q.Limit, 200)
	st, err := s.news()
	if err != nil {
		return NewsView{}, err
	}

	var subjects []news.Subject
	for _, star := range u.Stars() {
		subjects = append(subjects, news.Subject{ID: star.ID, Name: star.Name, Level: star.Level, Tags: star.Tags})
	}
	// Sub-theme names count too: an article about Kubernetes matters to someone with a
	// Kubernetes sub-theme even if no subject inside it is called that. Its level is the best
	// level reached inside it.
	for _, g := range u.Galaxies {
		for _, c := range g.Clusters {
			best := 0
			for _, st := range c.Stars {
				best = max(best, st.Level)
			}
			subjects = append(subjects, news.Subject{ID: c.ID, Name: c.Name, Level: best, Kind: "cluster"})
		}
	}
	m := news.NewMatcher(subjects)

	out := NewsView{Items: []NewsItem{}, Sources: []NewsSource{}}
	for _, it := range st.Items() {
		if !q.Since.IsZero() && it.Published.Before(q.Since) {
			continue
		}
		matches := m.Match(it.Title + " " + it.Summary)
		if matches == nil {
			matches = []news.Match{}
		}
		if !inNewsScope(q.Scope, it.Scope, matches) {
			continue
		}
		gid, _, _ := strings.Cut(it.Scope, "/")
		g := u.Galaxy(gid)
		if g == nil {
			continue
		}
		if len(matches) == 0 && !q.All && !g.Discovery.News {
			out.Hidden++
			continue
		}
		if len(out.Items) < q.Limit {
			out.Items = append(out.Items, NewsItem{
				ID: it.ID, Title: it.Title, Link: it.Link, Summary: it.Summary, Source: it.Source,
				Scope: it.Scope, ScopeName: scopeName(u, it.Scope), Galaxy: gid, Published: it.Published, Subjects: matches,
			})
		}
	}
	state := map[string]news.FeedState{}
	for _, f := range st.Feeds() {
		state[f.URL] = f
	}
	for _, f := range s.Feeds() {
		if q.Scope != "" && f.Scope != q.Scope && !strings.HasPrefix(f.Scope, q.Scope+"/") {
			continue
		}
		src := NewsSource{URL: f.URL, Name: f.Name, Scope: f.Scope, ScopeName: scopeName(u, f.Scope)}
		if fs, ok := state[f.URL]; ok {
			if !fs.LastOK.IsZero() {
				t := fs.LastOK
				src.LastOK = &t
			}
			src.Error = fs.Error
		}
		out.Sources = append(out.Sources, src)
	}
	return out, nil
}

// An item belongs to a scope when its feed is listed there or below, or when it mentions a
// subject of that scope: a Kubernetes feed item about Cilium also shows under "Réseau".
func inNewsScope(scope, itemScope string, matches []news.Match) bool {
	if scope == "" || itemScope == scope || strings.HasPrefix(itemScope, scope+"/") {
		return true
	}
	for _, m := range matches {
		if strings.HasPrefix(m.ID, scope+"/") {
			return true
		}
	}
	return false
}

func scopeName(u *kb.Universe, id string) string {
	if c := u.Cluster(id); c != nil {
		return c.Galaxy.Name + " › " + c.Name
	}
	if g := u.Galaxy(id); g != nil {
		return g.Name
	}
	return id
}

// ParseSince accepts a date (2026-11-09), an RFC 3339 time or a span back from now (24h, 7d).
func ParseSince(v string, now time.Time) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	if n := len(v); n > 1 && v[n-1] == 'd' {
		if days, err := strconv.Atoi(v[:n-1]); err == nil && days >= 0 {
			return now.AddDate(0, 0, -days), nil
		}
	}
	if d, err := time.ParseDuration(v); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("since %q: want 2006-01-02, RFC 3339, 24h or 7d: %w", v, ErrInvalid)
}
