package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListWorkspaces struct {
	DB *surreal.Client
}

type ListWorkspacesOutput struct {
	Workspaces []WorkspaceSummary `json:"workspaces"`
}

type WorkspaceSummary struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	NodeID   string `json:"nodeId"`
	NodeName string `json:"nodeName,omitempty"`
	DenID    string `json:"denId,omitempty"`
	DenName  string `json:"denName,omitempty"`
}

type ListWorkspacesInput struct {
	NodeID string `json:"nodeId,omitempty" jsonschema:"optional node identifier to filter by"`
	DenID  string `json:"denId,omitempty" jsonschema:"optional den identifier to filter by"`
}

func (l *ListWorkspaces) List(ctx context.Context, _ *mcp.CallToolRequest, input ListWorkspacesInput) (*mcp.CallToolResult, ListWorkspacesOutput, error) {
	if l == nil || l.DB == nil {
		return nil, ListWorkspacesOutput{}, fmt.Errorf("surreal client not configured")
	}

	type row struct {
		ID       string `json:"id"`
		Path     string `json:"path"`
		NodeID   string `json:"node_id"`
		NodeName string `json:"node_name"`
		Den      *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"den"`
	}

	baseQuery := `
SELECT meta::id(id) AS id,
       path,
       meta::id(node) AS node_id,
       node.name AS node_name,
       (SELECT {
            id: meta::id(in),
            name: in.name
        } FROM den_has_workspace WHERE out = id LIMIT 1)[0] AS den
FROM workspace
`

	var (
		filters []string
		vars    = map[string]any{}
	)

	if node := strings.TrimSpace(input.NodeID); node != "" {
		filters = append(filters, "meta::id(node) = $node_id")
		vars["node_id"] = node
	}

	if den := strings.TrimSpace(input.DenID); den != "" {
		filters = append(filters, "id IN (SELECT out FROM den_has_workspace WHERE in = type::thing('den', $den_id))")
		vars["den_id"] = den
	}

	if len(filters) > 0 {
		baseQuery += "WHERE " + strings.Join(filters, " AND ") + "\n"
	}

	baseQuery += "ORDER BY path ASC\n"

	rows, err := surreal.Query[row](ctx, l.DB, baseQuery, vars)
	if err != nil {
		return nil, ListWorkspacesOutput{}, fmt.Errorf("list workspaces: %w", err)
	}

	out := make([]WorkspaceSummary, 0, len(rows))
	for _, r := range rows {
		summary := WorkspaceSummary{
			ID:       r.ID,
			Path:     r.Path,
			NodeID:   r.NodeID,
			NodeName: r.NodeName,
		}
		if r.Den != nil {
			summary.DenID = r.Den.ID
			summary.DenName = r.Den.Name
		}
		out = append(out, summary)
	}

	return nil, ListWorkspacesOutput{Workspaces: out}, nil
}
