package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

// addAgentTools registers the read-only views that unattended agents need: what to review, what
// slipped, what changed, what waits in the inbox.
func addAgentTools(s *mcp.Server, svc *app.Service) {
	type dueIn struct {
		Scope string `json:"scope,omitempty" jsonschema:"theme, sub-theme or subject id; empty for everything"`
		Limit int    `json:"limit,omitempty" jsonschema:"max cards returned, default 15, at most 50; total counts every due card"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "due", Description: "Review cards due now under a scope, overdue first then new ones, with question and answer. Use it to quiz the user; grading happens in the app, not here."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in dueIn) (*mcp.CallToolResult, any, error) {
			v, err := svc.DueFor(in.Scope, in.Limit)
			if err != nil {
				return failure(err)
			}
			return text(v)
		})

	type riskIn struct {
		Scope string `json:"scope,omitempty" jsonschema:"theme or sub-theme id; empty for everything"`
		Limit int    `json:"limit,omitempty" jsonschema:"max subjects, default 8, at most 30"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "at_risk", Description: "Known subjects (level 2 or more) most likely forgotten: long since the last review compared with the theme's review_after, or with overdue cards. Each has a reason in French. Use it to pick what to quiz in a teach-back session."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in riskIn) (*mcp.CallToolResult, any, error) {
			v, err := svc.AtRisk(in.Scope, in.Limit)
			if err != nil {
				return failure(err)
			}
			return text(v)
		})

	type recentIn struct {
		Days int `json:"days,omitempty" jsonschema:"window in days, default 7, at most 365"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "recent", Description: "What moved in the last days: commits on the knowledge repo (subject line, files) and reviews per subject with how many were failed (again). Use it for a weekly recap or before suggesting work."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in recentIn) (*mcp.CallToolResult, any, error) {
			v, err := svc.Recent(ctx, in.Days)
			if err != nil {
				return failure(err)
			}
			return text(v)
		})

	type inboxIn struct {
		Limit int `json:"limit,omitempty" jsonschema:"max notes, default 20, at most 50; total counts every note"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "inbox", Description: "Notes captured for later triage, oldest first, each cut to 600 characters, with their file path. To place a note, propose() the target subject file and the same note path with delete=true, in one proposal."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in inboxIn) (*mcp.CallToolResult, any, error) {
			v, err := svc.InboxFor(in.Limit)
			if err != nil {
				return failure(err)
			}
			return text(v)
		})
}
