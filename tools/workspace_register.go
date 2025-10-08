package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type WorkspaceRegister struct {
	DB *surreal.Client
}

type WorkspaceRegisterInput struct {
	WorkspaceID string `json:"workspaceId" jsonschema:"stable identifier for workspace"`
	Path        string `json:"path" jsonschema:"absolute path to workspace root"`
	NodeID      string `json:"nodeId,omitempty" jsonschema:"optional node id to relate via on_node"`
}

type WorkspaceRegisterOutput struct {
	Workspace string `json:"workspace"`
	Node      string `json:"node,omitempty"`
}

func (w *WorkspaceRegister) Register(ctx context.Context, _ *mcp.CallToolRequest, input WorkspaceRegisterInput) (*mcp.CallToolResult, WorkspaceRegisterOutput, error) {
	if input.WorkspaceID == "" || input.Path == "" {
		return nil, WorkspaceRegisterOutput{}, fmt.Errorf("workspaceId and path are required")
	}
	if strings.TrimSpace(input.NodeID) == "" {
		return nil, WorkspaceRegisterOutput{}, fmt.Errorf("nodeId is required (schema asserts workspace.node != NONE)")
	}
	ws := fmt.Sprintf("type::thing('workspace', '%s')", input.WorkspaceID)
	node := fmt.Sprintf("type::thing('node', '%s')", input.NodeID)
	var stmts []string
	// Ensure node exists minimally (name is required by schema)
	stmts = append(stmts, fmt.Sprintf("UPSERT %s SET name = %s;", node, sLit(input.NodeID)))
	// Create or update workspace with node set
	stmts = append(stmts, fmt.Sprintf("UPSERT %s SET path = %s, node = %s, vcs = '', rev = '', content_sha = '';", ws, sLit(input.Path), node))
	// Relation
	stmts = append(stmts, fmt.Sprintf("RELATE (%s)->on_node->(%s);", ws, node))
	if err := w.DB.Exec(ctx, stmts); err != nil {
		return nil, WorkspaceRegisterOutput{}, err
	}
	return nil, WorkspaceRegisterOutput{Workspace: input.WorkspaceID, Node: input.NodeID}, nil
}

func sLit(s string) string {
	// Escape backslashes and single quotes for Surreal string literal
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return "'" + s + "'"
}
