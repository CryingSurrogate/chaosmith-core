package tools

import (
	"context"
	"fmt"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListNodes struct {
	DB *surreal.Client
}

type ListNodesOutput struct {
	Nodes []NodeSummary `json:"nodes" jsonschema:"registered nodes"`
}

type NodeSummary struct {
	ID     string   `json:"id" jsonschema:"node record id"`
	Name   string   `json:"name" jsonschema:"display name"`
	Kind   string   `json:"kind,omitempty" jsonschema:"node kind (pc, vm, etc.)"`
	OS     string   `json:"os,omitempty" jsonschema:"operating system"`
	CPU    string   `json:"cpu,omitempty" jsonschema:"cpu model"`
	RAMGB  int      `json:"ramGb,omitempty" jsonschema:"RAM in GB"`
	Labels []string `json:"labels,omitempty" jsonschema:"free-form labels"`
}

func (l *ListNodes) List(ctx context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, ListNodesOutput, error) {
	if l == nil || l.DB == nil {
		return nil, ListNodesOutput{}, fmt.Errorf("surreal client not configured")
	}

	type nodeRow struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Kind   string   `json:"kind"`
		OS     string   `json:"os"`
		CPU    string   `json:"cpu"`
		RAMGB  int      `json:"ram_gb"`
		Labels []string `json:"labels"`
	}

	const q = `
SELECT meta::id(id) AS id, name, kind, os, cpu, ram_gb, labels
FROM node
ORDER BY name ASC
`

	rows, err := surreal.Query[nodeRow](ctx, l.DB, q, nil)
	if err != nil {
		return nil, ListNodesOutput{}, fmt.Errorf("list nodes: %w", err)
	}

	summaries := make([]NodeSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, NodeSummary{
			ID:     row.ID,
			Name:   row.Name,
			Kind:   row.Kind,
			OS:     row.OS,
			CPU:    row.CPU,
			RAMGB:  row.RAMGB,
			Labels: row.Labels,
		})
	}

	return nil, ListNodesOutput{Nodes: summaries}, nil
}
