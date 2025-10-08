package tools

import (
	"context"

	"github.com/CryingSurrogate/chaosmith-core/internal/indexer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// L1IndexerTools exposes MCP handlers for workspace indexing.
type L1IndexerTools struct {
	Engine *indexer.Indexer
}

// IndexWorkspaceInput contains required fields for L1 steps.
type IndexWorkspaceInput struct {
	WorkspaceRoot string `json:"workspaceRoot" jsonschema:"absolute path to the workspace root"`
	WorkspaceID   string `json:"workspaceId" jsonschema:"stable workspace identifier"`
	RunID         string `json:"runId,omitempty" jsonschema:"optional deterministic run id"`
}

// IndexWorkspaceOutput wraps the run report.
type IndexWorkspaceOutput struct {
	Run *indexer.RunReport `json:"run,omitempty"`
}

// Scan handles index.workspace.scan.
func (l *L1IndexerTools) Scan(ctx context.Context, _ *mcp.CallToolRequest, input IndexWorkspaceInput) (*mcp.CallToolResult, IndexWorkspaceOutput, error) {
	report, err := l.Engine.Scan(ctx, indexer.WorkspaceRequest{
		WorkspaceRoot: input.WorkspaceRoot,
		WorkspaceID:   input.WorkspaceID,
		RunID:         input.RunID,
	})
	out := IndexWorkspaceOutput{Run: report}
	return nil, out, err
}

// Embed handles index.workspace.embed.
func (l *L1IndexerTools) Embed(ctx context.Context, _ *mcp.CallToolRequest, input IndexWorkspaceInput) (*mcp.CallToolResult, IndexWorkspaceOutput, error) {
	report, err := l.Engine.Embed(ctx, indexer.WorkspaceRequest{
		WorkspaceRoot: input.WorkspaceRoot,
		WorkspaceID:   input.WorkspaceID,
		RunID:         input.RunID,
	})
	out := IndexWorkspaceOutput{Run: report}
	return nil, out, err
}

// All orchestrates the full pipeline.
func (l *L1IndexerTools) All(ctx context.Context, _ *mcp.CallToolRequest, input IndexWorkspaceInput) (*mcp.CallToolResult, IndexWorkspaceOutput, error) {
	report, err := l.Engine.All(ctx, indexer.WorkspaceRequest{
		WorkspaceRoot: input.WorkspaceRoot,
		WorkspaceID:   input.WorkspaceID,
		RunID:         input.RunID,
	})
	out := IndexWorkspaceOutput{Run: report}
	return nil, out, err
}
