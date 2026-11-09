package news_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/news"
)

const rss = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><title>Kubernetes Blog</title>
<item><title>Cilium 1.18 &amp; Gateway API</title><link>https://example.com/cilium</link><guid>c-1</guid>
<pubDate>Mon, 02 Nov 2026 10:00:00 +0000</pubDate><description>&lt;p&gt;New &lt;b&gt;eBPF&lt;/b&gt; datapath&amp;nbsp;features.&lt;/p&gt;</description></item>
<item><title>Relative link</title><link>/posts/relative</link><pubDate>Tue, 3 Nov 2026 10:00:00 GMT</pubDate></item>
<item><title>Script link</title><link>javascript:alert(1)</link></item>
</channel></rss>`

const atom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>The Go Blog</title>
<entry><title>Go 1.27 is released</title><id>tag:go.dev,2026:go1.27</id>
<link rel="alternate" href="https://go.dev/blog/go1.27"/><link rel="self" href="https://go.dev/self"/>
<updated>2026-11-05T12:00:00Z</updated><summary type="html">Range over &lt;i&gt;functions&lt;/i&gt;.</summary></entry>
</feed>`

const rdf = `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel><title>Old style</title></channel>
<item><title>NixOS 26.05</title><link>https://nixos.org/blog/26.05</link><dc:date>2026-05-31T00:00:00Z</dc:date></item>
</rdf:RDF>`

func TestParse(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://example.com/feed.xml")
	tests := []struct {
		name      string
		doc       string
		title     string
		links     []string
		firstDate time.Time
	}{
		{"reads RSS 2.0, resolves relative links and drops non-http ones", rss, "Kubernetes Blog",
			[]string{"https://example.com/cilium", "https://example.com/posts/relative"}, time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)},
		{"reads Atom and prefers the alternate link", atom, "The Go Blog",
			[]string{"https://go.dev/blog/go1.27"}, time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)},
		{"reads RSS 1.0 with Dublin Core dates", rdf, "Old style",
			[]string{"https://nixos.org/blog/26.05"}, time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			title, entries, err := news.Parse(strings.NewReader(tt.doc), base)

			require.NoError(t, err)
			assert.Equal(t, tt.title, title)
			var links []string
			for _, e := range entries {
				links = append(links, e.Link)
			}
			assert.Equal(t, tt.links, links)
			assert.Equal(t, tt.firstDate, entries[0].Published)
		})
	}
}

func TestParse_StripsHTMLAndEntitiesFromTitlesAndSummaries(t *testing.T) {
	_, entries, err := news.Parse(strings.NewReader(rss), nil)

	require.NoError(t, err)
	assert.Equal(t, "Cilium 1.18 & Gateway API", entries[0].Title)
	assert.Equal(t, "New eBPF datapath features.", entries[0].Summary)
}

func TestParse_KeepsTheItemLink_WhenTheItemAlsoHasAnAtomSelfLink(t *testing.T) {
	doc := `<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom"><channel><title>x</title>
<item><title>Both links</title><atom:link rel="self" href="https://example.com/self"/><link>https://example.com/post</link></item>
</channel></rss>`

	_, entries, err := news.Parse(strings.NewReader(doc), nil)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "https://example.com/post", entries[0].Link)
}

func TestParse_DecodesLatin1AndWindows1252Feeds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		charset string
		raw     []byte
		want    string
	}{
		{"ISO-8859-1", "ISO-8859-1", []byte{'C', 'a', 'f', 0xe9}, "Café"},
		{"Windows-1252 smart quotes and euro", "windows-1252", []byte{0x93, '5', ' ', 0x80, 0x94}, "“5 €”"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := append([]byte(`<?xml version="1.0" encoding="`+tt.charset+`"?><rss version="2.0"><channel><title>x</title><item><title>`), tt.raw...)
			doc = append(doc, []byte(`</title><link>https://example.com/a</link></item></channel></rss>`)...)

			_, entries, err := news.Parse(strings.NewReader(string(doc)), nil)

			require.NoError(t, err)
			assert.Equal(t, tt.want, entries[0].Title)
		})
	}
}

func TestParse_RejectsAnHTMLPage(t *testing.T) {
	_, _, err := news.Parse(strings.NewReader("<html><body>nope</body></html>"), nil)

	assert.ErrorIs(t, err, news.ErrNotAFeed)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func fetcher(c *clock) *news.Fetcher {
	f := news.NewFetcher()
	f.Now = c.now
	return f
}

func TestRun_StoresNewItemsOnce_AndUsesConditionalGet(t *testing.T) {
	var conditional atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(rss))
	}))
	defer srv.Close()
	st, err := news.Open(t.TempDir())
	require.NoError(t, err)
	c := &clock{time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)}
	feeds := []news.Feed{{URL: srv.URL + "/feed", Name: "K8s", Scope: "tech/k8s"}}

	first, err := fetcher(c).Run(context.Background(), st, feeds)
	require.NoError(t, err)
	second, err := fetcher(c).Run(context.Background(), st, feeds)
	require.NoError(t, err)

	assert.Equal(t, 2, first.NewItems)
	assert.Equal(t, 1, second.NotMod)
	assert.Equal(t, int32(1), conditional.Load())
	assert.Len(t, st.Items(), 2)
	assert.Equal(t, "tech/k8s", st.Items()[0].Scope)
}

func TestRun_BacksOffAFailingFeed(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()
	st, _ := news.Open(t.TempDir())
	c := &clock{time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)}
	feeds := []news.Feed{{URL: srv.URL, Scope: "tech"}}

	first, _ := fetcher(c).Run(context.Background(), st, feeds)
	c.t = c.t.Add(10 * time.Minute)
	soon, _ := fetcher(c).Run(context.Background(), st, feeds)
	c.t = c.t.Add(time.Hour)
	later, _ := fetcher(c).Run(context.Background(), st, feeds)

	assert.Equal(t, 1, first.Failed)
	assert.Equal(t, 1, soon.Skipped)
	assert.Equal(t, 1, later.Failed)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 2, st.Feeds()[0].Failures)
}

func TestRun_DoesNotBackOff_WhenTheFetchIsCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	st, _ := news.Open(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	rep, _ := fetcher(&clock{time.Now()}).Run(ctx, st, []news.Feed{{URL: srv.URL}})

	assert.Equal(t, 0, rep.Failed)
	assert.Equal(t, 0, st.Feeds()[0].Failures)
	assert.True(t, st.Feeds()[0].NextTry.IsZero())
}

func TestRun_DoesNotCountArchiveItemsAsNew(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rss))
	}))
	defer srv.Close()
	st, _ := news.Open(t.TempDir())
	c := &clock{time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)}

	rep, err := fetcher(c).Run(context.Background(), st, []news.Feed{{URL: srv.URL}})

	require.NoError(t, err)
	assert.Equal(t, 0, rep.NewItems)
	assert.Empty(t, st.Items())
}

func TestRun_RefusesARedirectToAFileURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()
	st, _ := news.Open(t.TempDir())

	rep, err := fetcher(&clock{time.Now()}).Run(context.Background(), st, []news.Feed{{URL: srv.URL}})

	require.NoError(t, err)
	assert.Equal(t, 1, rep.Failed)
	assert.Contains(t, rep.Errors[0], "scheme not allowed")
}

func TestRun_RefusesAFeedLargerThanTheLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rss))
	}))
	defer srv.Close()
	st, _ := news.Open(t.TempDir())
	f := fetcher(&clock{time.Now()})
	f.MaxBytes = 100

	rep, _ := f.Run(context.Background(), st, []news.Feed{{URL: srv.URL}})

	assert.Equal(t, 1, rep.Failed)
}

func TestRun_DropsOldItemsAndItemsOfRemovedFeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/go" {
			w.Write([]byte(atom))
			return
		}
		w.Write([]byte(rss))
	}))
	defer srv.Close()
	dir := t.TempDir()
	st, _ := news.Open(dir)
	c := &clock{time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)}
	_, err := fetcher(c).Run(context.Background(), st, []news.Feed{{URL: srv.URL + "/k8s"}, {URL: srv.URL + "/go"}})
	require.NoError(t, err)
	c.t = time.Date(2026, 12, 4, 0, 0, 0, 0, time.UTC)

	_, err = fetcher(c).Run(context.Background(), st, []news.Feed{{URL: srv.URL + "/go"}})
	require.NoError(t, err)
	reopened, err := news.Open(dir)
	require.NoError(t, err)

	require.Len(t, reopened.Items(), 1)
	assert.Equal(t, "Go 1.27 is released", reopened.Items()[0].Title)
}

func TestMatcher(t *testing.T) {
	t.Parallel()
	m := news.NewMatcher([]news.Subject{
		{ID: "tech/k8s/cilium", Name: "Cilium", Level: 4},
		{ID: "tech/k8s/helm", Name: "Helm et Kustomize", Level: 3},
		{ID: "tech/lang/go", Name: "Go", Level: 4, Tags: []string{"golang"}},
		{ID: "tech/lang/cpp", Name: "C++", Level: 2},
		{ID: "tech/obs/otel", Name: "Télémétrie", Level: 1},
	})
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"finds a subject by name, case-insensitively", "What's new in CILIUM 1.18", []string{"tech/k8s/cilium"}},
		{"finds each part of a compound name", "Kustomize overlays explained", []string{"tech/k8s/helm"}},
		{"ignores a two-letter name that is an ordinary word", "Go ahead and deploy", nil},
		{"finds a short name through a longer tag", "Golang generics in practice", []string{"tech/lang/go"}},
		{"keeps symbols that make a language name", "Modern C++ modules", []string{"tech/lang/cpp"}},
		{"folds accents", "La telemetrie sans douleur", []string{"tech/obs/otel"}},
		{"matches whole words only", "Ciliumesque is not a word", nil},
		{"orders matches by level", "Helm charts for Cilium", []string{"tech/k8s/cilium", "tech/k8s/helm"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := m.Match(tt.text)

			var ids []string
			for _, g := range got {
				ids = append(ids, g.ID)
			}
			assert.Equal(t, tt.want, ids)
		})
	}
}
