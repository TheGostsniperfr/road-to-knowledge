// Package news follows the RSS and Atom feeds the user lists in their themes. Fetched items are a
// cache next to the knowledge, never knowledge themselves: nothing here is committed.
package news

import (
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Entry is one article as the feed describes it, before it becomes a stored Item.
type Entry struct {
	GUID      string
	Title     string
	Link      string
	Summary   string
	Published time.Time
}

type rssDoc struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Items []rssItem `xml:"item"` // RSS 1.0 (RDF) puts items next to the channel
}

type rssItem struct {
	Title       string    `xml:"title"`
	Links       []rssLink `xml:"link"`
	GUID        string    `xml:"guid"`
	PubDate     string    `xml:"pubDate"`
	Date        string    `xml:"http://purl.org/dc/elements/1.1/ date"`
	Description string    `xml:"description"`
	Content     string    `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

type atomDoc struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

var ErrNotAFeed = errors.New("not an RSS or Atom feed")

// Parse reads RSS 2.0, RSS 1.0 or Atom. Relative links resolve against base, and entries without an
// http(s) link are dropped: the UI only ever renders links it can open safely.
func Parse(r io.Reader, base *url.URL) (title string, entries []Entry, err error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", nil, fmt.Errorf("parse: %w", err)
	}
	root, err := rootName(data)
	if err != nil {
		return "", nil, err
	}
	switch root {
	case "rss", "RDF":
		var doc rssDoc
		if err := unmarshal(data, &doc); err != nil {
			return "", nil, err
		}
		for _, it := range append(doc.Channel.Items, doc.Items...) {
			body := it.Description
			if body == "" {
				body = it.Content
			}
			date := it.PubDate
			if date == "" {
				date = it.Date
			}
			entries = append(entries, Entry{GUID: strings.TrimSpace(it.GUID), Title: it.Title, Link: rssHref(it.Links), Summary: body, Published: parseDate(date)})
		}
		title = doc.Channel.Title
	case "feed":
		var doc atomDoc
		if err := unmarshal(data, &doc); err != nil {
			return "", nil, err
		}
		for _, e := range doc.Entries {
			body := e.Summary
			if body == "" {
				body = e.Content
			}
			date := e.Published
			if date == "" {
				date = e.Updated
			}
			entries = append(entries, Entry{GUID: strings.TrimSpace(e.ID), Title: e.Title, Link: atomHref(e.Links), Summary: body, Published: parseDate(date)})
		}
		title = doc.Title
	default:
		return "", nil, fmt.Errorf("root <%s>: %w", root, ErrNotAFeed)
	}

	out := entries[:0]
	for _, e := range entries {
		e.Link = resolve(base, strings.TrimSpace(e.Link))
		if e.Link == "" {
			continue
		}
		e.Title = oneLine(stripHTML(e.Title))
		e.Summary = clip(oneLine(stripHTML(e.Summary)), 400)
		if e.Title == "" {
			e.Title = e.Link
		}
		out = append(out, e)
	}
	return oneLine(stripHTML(title)), out, nil
}

func rootName(data []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false
	dec.CharsetReader = charsetReader
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("parse: %w", ErrNotAFeed)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}

func unmarshal(data []byte, v any) error {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	// Real feeds use HTML entities (&nbsp;, &eacute;) and the odd unknown charset label; be lenient.
	// No HTMLAutoClose: it treats <link> as an empty HTML element and would drop every RSS link.
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = charsetReader
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	return nil
}

func atomHref(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "" || l.Rel == "alternate" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

func resolve(base *url.URL, link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}

var dateLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC3339Nano, time.RFC3339, time.RFC822Z, time.RFC822,
	"Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04 -0700", "2006-01-02T15:04:05", "2006-01-02",
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

var tags = regexp.MustCompile(`(?s)<[^>]*>`)

func stripHTML(s string) string { return html.UnescapeString(tags.ReplaceAllString(s, " ")) }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

type rssLink struct {
	XMLName xml.Name
	Text    string `xml:",chardata"`
}

const atomNS = "http://www.w3.org/2005/Atom"

// Many RSS feeds add <atom:link rel="self" href="..."/> to items. Both decode as "link", so
// take the first one outside the Atom namespace that has text.
func rssHref(links []rssLink) string {
	for _, l := range links {
		if l.XMLName.Space != atomNS && strings.TrimSpace(l.Text) != "" {
			return l.Text
		}
	}
	return ""
}

// charsetReader handles the two legacy encodings still common in feeds without pulling in
// golang.org/x/text. Any other label is read as UTF-8.
func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "iso-8859-1", "latin1", "latin-1", "iso8859-1", "windows-1252", "cp1252":
		b, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		for _, c := range b {
			if c >= 0x80 && c < 0xa0 {
				sb.WriteRune(cp1252[c-0x80])
				continue
			}
			sb.WriteRune(rune(c))
		}
		return strings.NewReader(sb.String()), nil
	}
	return in, nil
}

// Windows-1252 differs from ISO-8859-1 only in 0x80-0x9F, where Latin-1 has control codes
// no feed uses, so one table serves both labels.
var cp1252 = [32]rune{
	'€', '\u0081', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '\u008d', 'Ž', '\u008f',
	'\u0090', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '\u009d', 'ž', 'Ÿ',
}
