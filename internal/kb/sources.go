package kb

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// CheckFeedURL accepts only plain http(s) feed addresses. Credentials in a URL would be committed
// with the knowledge repo, and any other scheme (file:, gopher:) has no business in a feed list.
func CheckFeedURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("not a URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q: only http and https feeds are fetched", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	if u.User != nil {
		return errors.New("credentials in the URL would be committed with the repo")
	}
	return nil
}

func validSources(path string, in []Source, report func(path, format string, args ...any)) []Source {
	var out []Source
	seen := map[string]bool{}
	for _, s := range in {
		s.URL = strings.TrimSpace(s.URL)
		if err := CheckFeedURL(s.URL); err != nil {
			report(path, "source %q: %v", s.URL, err)
			continue
		}
		if seen[s.URL] {
			continue
		}
		seen[s.URL] = true
		out = append(out, s)
	}
	return out
}
