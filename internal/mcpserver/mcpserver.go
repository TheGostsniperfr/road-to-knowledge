// Package mcpserver exposes the knowledge base to agents. Reads are cheap and scoped; writes either go
// to inbox/ or to a proposals/* branch, never to the main branch.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
)

const instructions = `Road to Knowledge is the user's map of what they know: galaxies (themes) > sub-themes > subjects (level 0-5, 0 = fog, never studied) > notions.
Start with profile(), then subgraph() or search() to locate subjects, node() for a brief, fragments() only when you need the full fiche.
To suggest what to learn next, call frontier(): it follows the user's discovery budget per theme and explains each suggestion.
Never load everything. Writes: capture() drops a note in the inbox; propose() commits Markdown files on a proposals/<slug> branch that the user reviews.
Subject files live at galaxies/<theme>/<sub-theme>/<subject>.md with YAML frontmatter (name, level, state, links, evidence, tags); "## heading" sections are notions; "question :: answer" lines are review cards.`

func New(svc *app.Service, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "road-to-knowledge", Version: version}, &mcp.ServerOptions{Instructions: instructions})

	type none struct{}
	mcp.AddTool(s, &mcp.Tool{Name: "profile", Description: "Summary of the user's knowledge: themes with average level, strongest subjects, work in progress, frontier, review backlog. Call this first."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
			return text(svc.Profile())
		})

	type subgraphIn struct {
		ID    string `json:"id,omitempty" jsonschema:"theme, sub-theme or subject id; empty for the whole universe"`
		Depth int    `json:"depth,omitempty" jsonschema:"levels below the node, 1 to 3, default 1"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "subgraph", Description: "Names, levels and states under a node. Use it to browse a theme without reading fiches."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in subgraphIn) (*mcp.CallToolResult, any, error) {
			if in.Depth == 0 {
				in.Depth = 1
			}
			n, err := svc.Subgraph(in.ID, in.Depth)
			if err != nil {
				return failure(err)
			}
			return text(n)
		})

	type idIn struct {
		ID string `json:"id" jsonschema:"subject id, e.g. lang/japonais/kana"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "node", Description: "Brief of one subject: level, state, last review, summary, notions, wormholes, evidence."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, any, error) {
			n, err := svc.Node(in.ID)
			if err != nil {
				return failure(err)
			}
			return text(n)
		})

	type fragIn struct {
		ID      string `json:"id" jsonschema:"subject id"`
		Section string `json:"section,omitempty" jsonschema:"one notion (## heading) to read instead of the whole fiche"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "fragments", Description: "Markdown of a subject's fiche, or of one notion in it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in fragIn) (*mcp.CallToolResult, any, error) {
			md, err := svc.Fragments(in.ID, in.Section)
			if err != nil {
				return failure(err)
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: md}}}, nil, nil
		})

	type searchIn struct {
		Query string `json:"query" jsonschema:"words to find in names and fiches; prefixes match"`
		Limit int    `json:"limit,omitempty" jsonschema:"max results, default 10"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "search", Description: "Full-text search over subjects, best matches first, with a snippet."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
			hits, err := svc.Search(ctx, in.Query, in.Limit)
			if err != nil {
				return failure(err)
			}
			return text(hits)
		})

	type frontierIn struct {
		Scope string `json:"scope,omitempty" jsonschema:"theme or sub-theme id; empty for the whole universe"`
		Limit int    `json:"limit,omitempty" jsonschema:"max suggestions, default 9, at most 50"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "frontier", Description: "What to learn next around a scope, split by the theme's discovery budget into near, two_hops and off_piste, each with a reason in French."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in frontierIn) (*mcp.CallToolResult, any, error) {
			f, err := svc.Frontier(in.Scope, in.Limit)
			if err != nil {
				return failure(err)
			}
			return text(f)
		})

	type captureIn struct {
		Text   string `json:"text" jsonschema:"the note, link or idea to keep"`
		Source string `json:"source,omitempty" jsonschema:"where it comes from, e.g. claude-code or an article title"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "capture", Description: "Save a quick note in the inbox for later triage. Does not touch the map."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in captureIn) (*mcp.CallToolResult, any, error) {
			if in.Source == "" {
				in.Source = "mcp"
			}
			p, err := svc.Capture(ctx, in.Text, in.Source)
			if err != nil {
				return failure(err)
			}
			return text(map[string]string{"saved": p})
		})

	type proposeFile struct {
		Path    string `json:"path" jsonschema:"relative path under galaxies/ or inbox/, ending in .md"`
		Content string `json:"content" jsonschema:"full file content with YAML frontmatter"`
	}
	type proposeIn struct {
		Slug    string        `json:"slug" jsonschema:"branch name suffix, e.g. fiche-kana; reusing a slug adds a commit to that branch"`
		Message string        `json:"message" jsonschema:"conventional commit message"`
		Files   []proposeFile `json:"files"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "propose", Description: "Create or replace subject files on branch proposals/<slug>. The user reviews and merges; nothing reaches main directly."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in proposeIn) (*mcp.CallToolResult, any, error) {
			files := make([]gitx.FileChange, len(in.Files))
			for i, f := range in.Files {
				files[i] = gitx.FileChange{Path: f.Path, Content: f.Content}
			}
			branch, err := svc.Propose(ctx, in.Slug, in.Message, files)
			if err != nil {
				return failure(err)
			}
			return text(map[string]string{"branch": branch, "review": "git diff main..." + branch})
		})

	return s
}

func text(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("encode: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

func failure(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
}
