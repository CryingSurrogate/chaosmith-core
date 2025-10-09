package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
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
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return nil, WorkspaceRegisterOutput{}, fmt.Errorf("path must not be blank")
	}

	data := map[string]any{
		"path":        path,
		"node":        surrealmodels.NewRecordID("node", strings.TrimSpace(input.NodeID)),
		"vcs":         "",
		"rev":         "",
		"content_sha": "",
	}

	if err := w.DB.UpsertRecord(ctx, "workspace", input.WorkspaceID, data); err != nil {
		return nil, WorkspaceRegisterOutput{}, fmt.Errorf("upsert workspace: %w", err)
	}

	if err := w.DB.Relate(ctx, "workspace", input.WorkspaceID, "on_node", "node", input.NodeID, nil); err != nil {
		return nil, WorkspaceRegisterOutput{}, fmt.Errorf("relate workspace to node: %w", err)
	}

	return nil, WorkspaceRegisterOutput{Workspace: input.WorkspaceID, Node: input.NodeID}, nil
}
