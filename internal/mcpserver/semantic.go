package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

type neighborsIn struct {
	ID    string `json:"id" jsonschema:"subject id, e.g. tech/kubernetes/argo-cd"`
	Limit int    `json:"limit,omitempty" jsonschema:"max subjects, default 8, at most 30"`
}

func addSemantic(s *mcp.Server, svc *app.Service) {
	mcp.AddTool(s, &mcp.Tool{Name: "neighbors", Description: "Subjects closest in meaning to a subject that are not linked to it yet, with a similarity score (higher is closer; compare scores within one answer only). Use it to suggest links, to place a new note next to the right subject, or to find what to read next. Fails while search by meaning is not ready: then use search() or subgraph()."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in neighborsIn) (*mcp.CallToolResult, any, error) {
			n, err := svc.Neighbors(in.ID, in.Limit)
			if err != nil {
				return failure(err)
			}
			return text(n)
		})
}
