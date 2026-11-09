package news

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Feed is a source as the knowledge repo declares it: Scope is the theme or sub-theme that lists it.
type Feed struct {
	URL   string
	Name  string
	Scope string
}

type Item struct {
	ID        string    `json:"id"`
	Feed      string    `json:"feed"`
	Source    string    `json:"source"`
	Scope     string    `json:"scope"`
	Title     string    `json:"title"`
	Link      string    `json:"link"`
	Summary   string    `json:"summary,omitempty"`
	Published time.Time `json:"published"`
	Fetched   time.Time `json:"fetched"`
}

type FeedState struct {
	URL          string    `json:"url"`
	Name         string    `json:"name"`
	Scope        string    `json:"scope"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	LastOK       time.Time `json:"last_ok"`
	NextTry      time.Time `json:"next_try"`
	Failures     int       `json:"failures,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// Store is the cache file .rtk/news/state.json. It is rebuilt from the feeds if deleted.
type Store struct {
	path  string
	mu    sync.Mutex
	feeds map[string]*FeedState
	items []Item
}

type stateFile struct {
	Feeds map[string]*FeedState `json:"feeds"`
	Items []Item                `json:"items"`
}

func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "state.json"), feeds: map[string]*FeedState{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("news: %w", err)
	}
	var st stateFile
	if err := json.Unmarshal(b, &st); err != nil {
		// A corrupt cache is not worth failing the server for: start over, the feeds refill it.
		return s, nil
	}
	if st.Feeds != nil {
		s.feeds = st.Feeds
	}
	s.items = st.Items
	return s, nil
}

func (s *Store) Items() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Item(nil), s.items...)
}

func (s *Store) Feeds() []FeedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]FeedState, 0, len(s.feeds))
	for _, f := range s.feeds {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

func (s *Store) save() error {
	s.mu.Lock()
	b, err := json.Marshal(stateFile{Feeds: s.feeds, Items: s.items})
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("news: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("news: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), "state-*.tmp")
	if err != nil {
		return fmt.Errorf("news: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(f.Name())
		return fmt.Errorf("news: %w", err)
	}
	f.Close()
	if err := os.Rename(f.Name(), s.path); err != nil {
		os.Remove(f.Name())
		return fmt.Errorf("news: %w", err)
	}
	return nil
}

type Fetcher struct {
	Client    *http.Client
	Now       func() time.Time
	Retain    time.Duration
	PerFeed   int
	MaxBytes  int64
	UserAgent string
	Parallel  int
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		Client: &http.Client{
			Timeout: 20 * time.Second,
			// The URLs come from the user's own repo, but a feed may still redirect anywhere.
			// Only http(s) hops, and not too many of them.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return fmt.Errorf("redirect to %s: scheme not allowed", req.URL.Scheme)
				}
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
		Now:       time.Now,
		Retain:    30 * 24 * time.Hour,
		PerFeed:   50,
		MaxBytes:  4 << 20,
		UserAgent: "road-to-knowledge/rtk (+https://github.com/TheGostsniperfr/road-to-knowledge)",
		Parallel:  4,
	}
}

type Report struct {
	Fetched  int      `json:"fetched"`
	NotMod   int      `json:"not_modified"`
	Skipped  int      `json:"skipped"`
	Failed   int      `json:"failed"`
	NewItems int      `json:"new_items"`
	Errors   []string `json:"errors,omitempty"`
}

// Run fetches every feed whose backoff has expired, merges new items and prunes old ones and the
// items of feeds the repo no longer lists.
func (f *Fetcher) Run(ctx context.Context, st *Store, feeds []Feed) (Report, error) {
	now := f.Now()
	type result struct {
		feed    Feed
		entries []Entry
		etag    string
		lastMod string
		notMod  bool
		err     error
	}
	st.mu.Lock()
	want := map[string]bool{}
	var todo []Feed
	var rep Report
	for _, fd := range feeds {
		want[fd.URL] = true
		fs := st.feeds[fd.URL]
		if fs == nil {
			fs = &FeedState{URL: fd.URL}
			st.feeds[fd.URL] = fs
		}
		fs.Name, fs.Scope = fd.Name, fd.Scope
		if now.Before(fs.NextTry) {
			rep.Skipped++
			continue
		}
		todo = append(todo, fd)
	}
	for u := range st.feeds {
		if !want[u] {
			delete(st.feeds, u)
		}
	}
	st.mu.Unlock()

	results := make(chan result)
	sem := make(chan struct{}, max(1, f.Parallel))
	var wg sync.WaitGroup
	for _, fd := range todo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			st.mu.Lock()
			etag, lastMod := st.feeds[fd.URL].ETag, st.feeds[fd.URL].LastModified
			st.mu.Unlock()
			r := result{feed: fd}
			r.entries, r.etag, r.lastMod, r.notMod, r.err = f.fetch(ctx, fd.URL, etag, lastMod)
			results <- r
		}()
	}
	go func() { wg.Wait(); close(results) }()

	st.mu.Lock()
	seen := map[string]bool{}
	for _, it := range st.items {
		seen[it.ID] = true
	}
	st.mu.Unlock()
	for r := range results {
		st.mu.Lock()
		fs := st.feeds[r.feed.URL]
		if r.err != nil && ctx.Err() != nil {
			// Cancelled by us (Ctrl-C, closed request): not the feed's fault, no backoff.
			rep.Skipped++
			st.mu.Unlock()
			continue
		}
		if r.err != nil {
			fs.Failures++
			fs.Error = r.err.Error()
			// 30 min, 1 h, 2 h... capped at a day: a dead feed must not be hammered on every tick.
			fs.NextTry = now.Add(min(30*time.Minute<<min(fs.Failures-1, 10), 24*time.Hour))
			rep.Failed++
			rep.Errors = append(rep.Errors, r.feed.URL+": "+r.err.Error())
			st.mu.Unlock()
			continue
		}
		fs.Failures, fs.Error, fs.NextTry, fs.LastOK = 0, "", time.Time{}, now
		if r.notMod {
			rep.NotMod++
			st.mu.Unlock()
			continue
		}
		fs.ETag, fs.LastModified = r.etag, r.lastMod
		rep.Fetched++
		for i, e := range r.entries {
			if i >= f.PerFeed {
				break
			}
			id := itemID(r.feed.URL, e)
			if seen[id] {
				continue
			}
			seen[id] = true
			pub := e.Published
			if pub.IsZero() || pub.After(now) {
				pub = now
			}
			// A feed that carries years of archive must not report them as new, only for the
			// prune below to drop them again.
			if now.Sub(pub) > f.Retain {
				continue
			}
			st.items = append(st.items, Item{ID: id, Feed: r.feed.URL, Source: r.feed.Name, Scope: r.feed.Scope, Title: e.Title, Link: e.Link, Summary: e.Summary, Published: pub, Fetched: now})
			rep.NewItems++
		}
		st.mu.Unlock()
	}

	st.mu.Lock()
	kept := st.items[:0]
	for _, it := range st.items {
		if want[it.Feed] && now.Sub(it.Published) <= f.Retain {
			if fs := st.feeds[it.Feed]; fs != nil {
				it.Source, it.Scope = fs.Name, fs.Scope
			}
			kept = append(kept, it)
		}
	}
	st.items = kept
	sort.SliceStable(st.items, func(i, j int) bool { return st.items[i].Published.After(st.items[j].Published) })
	st.mu.Unlock()
	if err := st.save(); err != nil {
		return rep, err
	}
	return rep, nil
}

func (f *Fetcher) fetch(ctx context.Context, raw, etag, lastMod string) (entries []Entry, newETag, newLastMod string, notMod bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", "", false, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}
	res, err := f.Client.Do(req)
	if err != nil {
		return nil, "", "", false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return nil, etag, lastMod, true, nil
	}
	if res.StatusCode != http.StatusOK {
		return nil, "", "", false, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	body := io.LimitReader(res.Body, f.MaxBytes+1)
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, "", "", false, err
	}
	if int64(len(data)) > f.MaxBytes {
		return nil, "", "", false, fmt.Errorf("feed larger than %d bytes", f.MaxBytes)
	}
	base, _ := url.Parse(res.Request.URL.String())
	_, entries, err = Parse(bytes.NewReader(data), base)
	if err != nil {
		return nil, "", "", false, err
	}
	return entries, res.Header.Get("ETag"), res.Header.Get("Last-Modified"), false, nil
}

func itemID(feed string, e Entry) string {
	key := e.GUID
	if key == "" {
		key = e.Link
	}
	h := sha1.Sum([]byte(feed + "|" + key))
	return hex.EncodeToString(h[:6])
}
