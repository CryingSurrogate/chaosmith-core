package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type FindFile struct {
	DB *surreal.Client
}

type FindFileInput struct {
	WorkspaceID string `json:"workspaceId" jsonschema:"workspace identifier"`
	Query       string `json:"query" jsonschema:"exact match or substring to look for"`
	MatchType   string `json:"matchType,omitempty" jsonschema:"exact | substring | prefix | suffix"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum number of results to return"`
}

type FindFileOutput struct {
	Results []FindFileResult `json:"results" jsonschema:"matching files"`
}

type FindFileResult struct {
	RelPath string `json:"relpath" jsonschema:"path relative to workspace root"`
	Lang    string `json:"lang,omitempty" jsonschema:"language hint"`
	Size    int64  `json:"size" jsonschema:"file size in bytes"`
	SHA     string `json:"sha" jsonschema:"content hash"`
}

func (f *FindFile) Search(ctx context.Context, _ *mcp.CallToolRequest, input FindFileInput) (*mcp.CallToolResult, FindFileOutput, error) {
	if f == nil || f.DB == nil {
		return nil, FindFileOutput{}, fmt.Errorf("surreal client not configured")
	}
	wsID := strings.TrimSpace(input.WorkspaceID)
	if wsID == "" {
		return nil, FindFileOutput{}, fmt.Errorf("workspaceId is required")
	}
	q := strings.TrimSpace(input.Query)
	if q == "" {
		return nil, FindFileOutput{}, fmt.Errorf("query is required")
	}

	matchType := strings.ToLower(strings.TrimSpace(input.MatchType))
	if matchType == "" {
		matchType = "substring"
	}

	var (
		filter string
		vars   = map[string]any{
			"ws_id": wsID,
			"limit": clampLimit(input.Limit, 100),
		}
	)

	switch matchType {
	case "exact":
		filter = "relpath = $query"
		vars["query"] = q
	case "prefix":
		filter = "string::begins_with(relpath, $query)"
		vars["query"] = q
	case "suffix":
		filter = "string::ends_with(relpath, $query)"
		vars["query"] = q
	case "substring":
		filter = "string::contains(relpath, $query)"
		vars["query"] = q
	default:
		return nil, FindFileOutput{}, fmt.Errorf("unsupported matchType %q", matchType)
	}

	const tmpl = `
SELECT relpath, lang, size, sha
FROM file
WHERE ws = type::thing('workspace', $ws_id) AND %s
ORDER BY relpath ASC
LIMIT $limit
`

	sql := fmt.Sprintf(tmpl, filter)

	type row struct {
		RelPath string `json:"relpath"`
		Lang    string `json:"lang"`
		Size    int64  `json:"size"`
		SHA     string `json:"sha"`
	}

	rows, err := surreal.Query[row](ctx, f.DB, sql, vars)
	if err != nil {
		return nil, FindFileOutput{}, fmt.Errorf("find files: %w", err)
	}

	results := make([]FindFileResult, 0, len(rows))
	for _, r := range rows {
		results = append(results, FindFileResult{
			RelPath: r.RelPath,
			Lang:    r.Lang,
			Size:    r.Size,
			SHA:     r.SHA,
		})
	}

	return nil, FindFileOutput{Results: results}, nil
}
