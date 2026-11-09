package mcpserver

import (
	"context"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

type newsIn struct {
	Scope string `json:"scope,omitempty" jsonschema:"theme or sub-theme id; empty for every theme"`
	Since string `json:"since,omitempty" jsonschema:"only items published after: 2026-11-09, RFC 3339, 24h or 7d; default 2d"`
	Limit int    `json:"limit,omitempty" jsonschema:"max items, default 20, at most 100"`
	All   bool   `json:"all,omitempty" jsonschema:"also return items that mention no known subject in themes where news discovery is off"`
}

type newsOutItem struct {
	Title     string   `json:"title"`
	Link      string   `json:"link"`
	Source    string   `json:"source"`
	Theme     string   `json:"theme"`
	Published string   `json:"published"`
	Subjects  []string `json:"subjects,omitempty"`
	Summary   string   `json:"summary,omitempty"`
}

func addNews(s *mcp.Server, svc *app.Service) {
	mcp.AddTool(s, &mcp.Tool{Name: "news", Description: "Recent articles from the feeds listed in the user's themes, newest first, each with the known subjects it mentions as \"Name (level N)\". Read-only: keep an item with capture()."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in newsIn) (*mcp.CallToolResult, any, error) {
			if in.Since == "" {
				in.Since = "2d"
			}
			since, err := app.ParseSince(in.Since, time.Now())
			if err != nil {
				return failure(err)
			}
			if in.Limit <= 0 {
				in.Limit = 20
			}
			v, err := svc.News(app.NewsQuery{Scope: in.Scope, Since: since, Limit: min(in.Limit, 100), All: in.All})
			if err != nil {
				return failure(err)
			}
			out := struct {
				Items  []newsOutItem `json:"items"`
				Hidden int           `json:"hidden_unmatched,omitempty"`
				Feeds  int           `json:"feeds"`
				Broken []string      `json:"broken_feeds,omitempty"`
			}{Items: []newsOutItem{}, Hidden: v.Hidden, Feeds: len(v.Sources)}
			for _, it := range v.Items {
				o := newsOutItem{Title: it.Title, Link: it.Link, Source: it.Source, Theme: it.ScopeName, Published: it.Published.Format("2006-01-02")}
				for _, m := range it.Subjects {
					if m.Kind == "cluster" {
						o.Subjects = append(o.Subjects, m.Name+" (sub-theme, best level "+strconv.Itoa(m.Level)+")")
						continue
					}
					o.Subjects = append(o.Subjects, m.Name+" (level "+strconv.Itoa(m.Level)+")")
				}
				if len([]rune(it.Summary)) > 200 {
					o.Summary = string([]rune(it.Summary)[:200]) + "…"
				} else {
					o.Summary = it.Summary
				}
				out.Items = append(out.Items, o)
			}
			for _, src := range v.Sources {
				if src.Error != "" {
					out.Broken = append(out.Broken, src.Name+": "+src.Error)
				}
			}
			return text(out)
		})
}
