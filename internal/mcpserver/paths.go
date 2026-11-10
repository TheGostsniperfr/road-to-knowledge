package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

type pathsIn struct {
	ID string `json:"id,omitempty" jsonschema:"a path id from the list; empty lists every path with its progress and next step"`
}

func addPaths(s *mcp.Server, svc *app.Service) {
	mcp.AddTool(s, &mcp.Tool{Name: "paths", Description: "Learning paths: the reading order of a book or a course (Genki, a video playlist) over the map. Without id: every path with done/total and the next recommended step. With id: every step with its status (done, current, todo), its URL or page, and the subjects or notions it covers. To move the cursor or add steps, send the edited paths/<id>.md through propose()."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in pathsIn) (*mcp.CallToolResult, any, error) {
			if in.ID == "" {
				return text(svc.Paths())
			}
			p, err := svc.Path(in.ID)
			if err != nil {
				return failure(err)
			}
			return text(p)
		})
}
