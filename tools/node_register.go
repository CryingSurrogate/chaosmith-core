package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type NodeRegister struct {
	DB *surreal.Client
}

type NodeRegisterInput struct {
	NodeID string   `json:"nodeId" jsonschema:"stable identifier for node"`
	Name   string   `json:"name,omitempty" jsonschema:"display name; defaults to nodeId"`
	Kind   string   `json:"kind,omitempty" jsonschema:"kind of node (pc, vm, arm, etc.)"`
	OS     string   `json:"os,omitempty" jsonschema:"operating system summary"`
	CPU    string   `json:"cpu,omitempty" jsonschema:"cpu model summary"`
	RAMGB  *int     `json:"ramGb,omitempty" jsonschema:"ram size in GB"`
	Labels []string `json:"labels,omitempty" jsonschema:"optional free-form labels"`
}

type NodeRegisterOutput struct {
	Node string `json:"node"`
}

func (n *NodeRegister) Register(ctx context.Context, _ *mcp.CallToolRequest, input NodeRegisterInput) (*mcp.CallToolResult, NodeRegisterOutput, error) {
	nodeID := strings.TrimSpace(input.NodeID)
	if nodeID == "" {
		return nil, NodeRegisterOutput{}, fmt.Errorf("nodeId is required")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = nodeID
	}

	data := map[string]any{
		"name": name,
	}
	if kind := strings.TrimSpace(input.Kind); kind != "" {
		data["kind"] = kind
	}
	if os := strings.TrimSpace(input.OS); os != "" {
		data["os"] = os
	}
	if cpu := strings.TrimSpace(input.CPU); cpu != "" {
		data["cpu"] = cpu
	}
	if input.RAMGB != nil {
		data["ram_gb"] = *input.RAMGB
	}
	if len(input.Labels) > 0 {
		labels := make([]string, 0, len(input.Labels))
		for _, label := range input.Labels {
			if trimmed := strings.TrimSpace(label); trimmed != "" {
				labels = append(labels, trimmed)
			}
		}
		if len(labels) > 0 {
			data["labels"] = labels
		}
	}

	if err := n.DB.UpsertRecord(ctx, "node", nodeID, data); err != nil {
		return nil, NodeRegisterOutput{}, fmt.Errorf("upsert node: %w", err)
	}

	return nil, NodeRegisterOutput{Node: nodeID}, nil
}
