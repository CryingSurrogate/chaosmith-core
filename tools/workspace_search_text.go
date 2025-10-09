package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type WorkspaceSearchText struct {
	DB *surreal.Client
}

type WorkspaceSearchTextInput struct {
	WorkspaceID   string `json:"workspaceId" jsonschema:"workspace identifier"`
	Query         string `json:"query" jsonschema:"exact text snippet to find"`
	CaseSensitive bool   `json:"caseSensitive,omitempty" jsonschema:"if true, match is case-sensitive"`
	Limit         int    `json:"limit,omitempty" jsonschema:"max number of matches (default 20)"`
	MaxFileBytes  int64  `json:"maxFileBytes,omitempty" jsonschema:"skip files larger than this many bytes (default 1048576)"`
}

type WorkspaceSearchTextOutput struct {
	Matches []TextMatch `json:"matches" jsonschema:"list of file matches"`
}

type TextMatch struct {
	RelPath    string `json:"relpath" jsonschema:"file path relative to workspace root"`
	LineNumber int    `json:"lineNumber" jsonschema:"line number of match"`
	Snippet    string `json:"snippet" jsonschema:"line containing the match"`
}

func (s *WorkspaceSearchText) Search(ctx context.Context, _ *mcp.CallToolRequest, input WorkspaceSearchTextInput) (*mcp.CallToolResult, WorkspaceSearchTextOutput, error) {
	if s == nil || s.DB == nil {
		return nil, WorkspaceSearchTextOutput{}, fmt.Errorf("surreal client not configured")
	}
	wsID := strings.TrimSpace(input.WorkspaceID)
	if wsID == "" {
		return nil, WorkspaceSearchTextOutput{}, fmt.Errorf("workspaceId is required")
	}
	query := input.Query
	if strings.TrimSpace(query) == "" {
		return nil, WorkspaceSearchTextOutput{}, fmt.Errorf("query is required")
	}

	maxBytes := input.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = 1 << 20 // 1 MiB
	}
	limit := clampLimit(input.Limit, 100)
	if limit == 0 {
		limit = 20
	}

	wsPath, err := s.lookupWorkspacePath(ctx, wsID)
	if err != nil {
		return nil, WorkspaceSearchTextOutput{}, err
	}

	files, err := s.listWorkspaceFiles(ctx, wsID)
	if err != nil {
		return nil, WorkspaceSearchTextOutput{}, err
	}

	caseSensitive := input.CaseSensitive
	searchNeedle := query
	if !caseSensitive {
		searchNeedle = strings.ToLower(query)
	}

	var matches []TextMatch
	for _, rel := range files {
		if len(matches) >= limit {
			break
		}
		fullPath := filepath.Join(wsPath, filepath.FromSlash(rel))
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > maxBytes {
			continue
		}
		content, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(content)
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 2*1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			lineForSearch := line
			if !caseSensitive {
				lineForSearch = strings.ToLower(line)
			}
			if strings.Contains(lineForSearch, searchNeedle) {
				matches = append(matches, TextMatch{
					RelPath:    rel,
					LineNumber: lineNo,
					Snippet:    strings.TrimSpace(line),
				})
				if len(matches) >= limit {
					break
				}
			}
		}
		content.Close()
		if len(matches) >= limit {
			break
		}
	}

	return nil, WorkspaceSearchTextOutput{Matches: matches}, nil
}

func (s *WorkspaceSearchText) lookupWorkspacePath(ctx context.Context, wsID string) (string, error) {
	type row struct {
		Path string `json:"path"`
	}
	const q = `
SELECT path FROM workspace WHERE id = type::thing('workspace', $ws_id) LIMIT 1
`
	rows, err := surreal.Query[row](ctx, s.DB, q, map[string]any{"ws_id": wsID})
	if err != nil {
		return "", fmt.Errorf("lookup workspace path: %w", err)
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].Path) == "" {
		return "", fmt.Errorf("workspace %s not found or missing path", wsID)
	}
	return rows[0].Path, nil
}

func (s *WorkspaceSearchText) listWorkspaceFiles(ctx context.Context, wsID string) ([]string, error) {
	type row struct {
		RelPath string `json:"relpath"`
	}
	const q = `
SELECT relpath FROM file WHERE ws = type::thing('workspace', $ws_id)
ORDER BY relpath ASC
`
	rows, err := surreal.Query[row](ctx, s.DB, q, map[string]any{"ws_id": wsID})
	if err != nil {
		return nil, fmt.Errorf("list workspace files: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.RelPath)
	}
	return out, nil
}
