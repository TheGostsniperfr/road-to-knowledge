package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

// addRestructureTool lets an agent reorganise the map. The server computes every file change from
// the current files, so an agent describes the intent and never deletes a subject by hand; the
// result is a proposal branch the user merges (ADR-0004, ADR-0014).
func addRestructureTool(s *mcp.Server, svc *app.Service) {
	type partIn struct {
		ID      string   `json:"id" jsonschema:"new subject id, or just its slug to stay in the same sub-theme"`
		Name    string   `json:"name,omitempty" jsonschema:"display name; default: the notion name, or the slug"`
		Notions []string `json:"notions" jsonschema:"names of the ## notions that move to this part, with the cards written under them"`
	}
	type restructureIn struct {
		Kind    string   `json:"kind" jsonschema:"move, split or merge"`
		ID      string   `json:"id,omitempty" jsonschema:"move and split: the subject (theme/sub-theme/subject) or, for move, a sub-theme (theme/sub-theme)"`
		To      string   `json:"to,omitempty" jsonschema:"move: the new id, or an existing sub-theme to move a subject into, or an existing theme to move a sub-theme into; same parent with a new slug renames"`
		Name    string   `json:"name,omitempty" jsonschema:"move: new display name; merge: name of the merged subject"`
		Parts   []partIn `json:"parts,omitempty" jsonschema:"split: the new subjects; the original keeps its summary, level and every notion not listed"`
		Sources []string `json:"sources,omitempty" jsonschema:"merge: subject ids to fold together"`
		Into    string   `json:"into,omitempty" jsonschema:"merge: one of the sources, or a new subject id; default the first source"`
		Slug    string   `json:"slug,omitempty" jsonschema:"proposal branch is proposals/<slug>; default restructure-<kind>-<id>"`
		DryRun  bool     `json:"dry_run,omitempty" jsonschema:"true: only return the plan (files moved, written, deleted; id renames), commit nothing"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "restructure", Description: "Reorganise subjects: move or rename a subject or sub-theme, split a subject's notions into new subjects, or merge subjects. Review history, links and old deep links follow the new ids; a merge keeps the highest level. Writes a proposals/<slug> branch for the user, never main. Call with dry_run first and show the plan."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in restructureIn) (*mcp.CallToolResult, any, error) {
			op := app.Op{Kind: in.Kind, ID: in.ID, To: in.To, Name: in.Name, Sources: in.Sources, Into: in.Into}
			for _, p := range in.Parts {
				op.Parts = append(op.Parts, app.SplitPart{ID: p.ID, Name: p.Name, Notions: p.Notions})
			}
			plan, err := svc.ProposeRestructure(ctx, op, in.Slug, in.DryRun)
			if err != nil {
				return failure(err)
			}
			return text(plan)
		})
}
