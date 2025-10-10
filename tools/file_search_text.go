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

type FileSearchText struct {
	DB *surreal.Client
}

type FileSearchTextInput struct {
	WorkspaceID   string `json:"workspaceId" jsonschema:"workspace identifier"`
	RelPath       string `json:"relpath" jsonschema:"file path relative to workspace root"`
	Query         string `json:"query" jsonschema:"exact text snippet to find"`
	CaseSensitive bool   `json:"caseSensitive,omitempty" jsonschema:"if true, match is case-sensitive"`
	Limit         int    `json:"limit,omitempty" jsonschema:"max matches to return (default 20)"`
}

type FileSearchTextOutput struct {
	Matches []TextMatch `json:"matches" jsonschema:"list of matches within the file"`
}

func (s *FileSearchText) Search(ctx context.Context, _ *mcp.CallToolRequest, input FileSearchTextInput) (*mcp.CallToolResult, FileSearchTextOutput, error) {
	matches := make([]TextMatch, 0, input.Limit)
	if s == nil || s.DB == nil {
		return nil, FileSearchTextOutput{Matches: matches}, fmt.Errorf("surreal client not configured")
	}
	wsID := strings.TrimSpace(input.WorkspaceID)
	if wsID == "" {
		return nil, FileSearchTextOutput{Matches: matches}, fmt.Errorf("workspaceId is required")
	}
	rel := strings.TrimSpace(input.RelPath)
	if rel == "" {
		return nil, FileSearchTextOutput{Matches: matches}, fmt.Errorf("relpath is required")
	}
	query := input.Query
	if strings.TrimSpace(query) == "" {
		return nil, FileSearchTextOutput{Matches: matches}, fmt.Errorf("query is required")
	}

	fsPath, err := s.resolveFilePath(ctx, wsID, rel)
	if err != nil {
		return nil, FileSearchTextOutput{Matches: matches}, err
	}

	limit := clampLimit(input.Limit, 100)
	if limit == 0 {
		limit = 20
	}

	caseSensitive := input.CaseSensitive
	searchNeedle := query
	if !caseSensitive {
		searchNeedle = strings.ToLower(query)
	}

	file, err := os.Open(fsPath)
	if err != nil {
		return nil, FileSearchTextOutput{Matches: matches}, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
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
	if err := scanner.Err(); err != nil {
		return nil, FileSearchTextOutput{Matches: matches}, fmt.Errorf("scan file: %w", err)
	}

	return nil, FileSearchTextOutput{Matches: matches}, nil
}

func (s *FileSearchText) resolveFilePath(ctx context.Context, wsID, rel string) (string, error) {
	type wsRow struct {
		Path string `json:"path"`
	}
	const wsQuery = `
SELECT path FROM workspace WHERE id = type::thing('workspace', $ws_id) LIMIT 1
`
	wsRows, err := surreal.Query[wsRow](ctx, s.DB, wsQuery, map[string]any{"ws_id": wsID})
	if err != nil {
		return "", fmt.Errorf("lookup workspace path: %w", err)
	}
	if len(wsRows) == 0 || strings.TrimSpace(wsRows[0].Path) == "" {
		return "", fmt.Errorf("workspace %s not found or missing path", wsID)
	}

	type fileRow struct {
		Count int `json:"count"`
	}
	const fileQuery = `
SELECT count() AS count FROM file
WHERE ws = type::thing('workspace', $ws_id) AND relpath = $rel
`
	fileRows, err := surreal.Query[fileRow](ctx, s.DB, fileQuery, map[string]any{"ws_id": wsID, "rel": rel})
	if err != nil {
		return "", fmt.Errorf("verify file: %w", err)
	}
	if len(fileRows) == 0 || fileRows[0].Count == 0 {
		return "", fmt.Errorf("file %s not found in workspace %s", rel, wsID)
	}

	wsPath := strings.TrimSpace(wsRows[0].Path)
	return filepath.Join(wsPath, filepath.FromSlash(rel)), nil
}
