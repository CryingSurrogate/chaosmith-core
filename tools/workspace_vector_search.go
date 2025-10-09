package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/embedder"
	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/surrealdb/surrealdb.go"
)

type WorkspaceVectorSearch struct {
	DB       *surreal.Client
	Embedder *embedder.Client
}

type WorkspaceVectorSearchInput struct {
	WorkspaceID string   `json:"workspaceId" jsonschema:"workspace identifier"`
	Query       string   `json:"query" jsonschema:"natural language query"`
	TopK        int      `json:"topK,omitempty" jsonschema:"number of results (default 5, max 50)"`
	ModelID     string   `json:"modelId,omitempty" jsonschema:"vector model slug override"`
	FileFilter  []string `json:"fileFilter,omitempty" jsonschema:"optional list of file relpaths to include"`
}

type WorkspaceVectorSearchOutput struct {
	Matches []WorkspaceVectorMatch `json:"matches" jsonschema:"ranked vector matches across workspace"`
}

type WorkspaceVectorMatch struct {
	Score      float64 `json:"score" jsonschema:"cosine similarity score"`
	File       string  `json:"file" jsonschema:"file relpath"`
	Start      int     `json:"start" jsonschema:"chunk start byte"`
	End        int     `json:"end" jsonschema:"chunk end byte"`
	TokenCount int     `json:"tokenCount" jsonschema:"chunk token count"`
	ContentSHA string  `json:"contentSha" jsonschema:"chunk content hash"`
}

func (s *WorkspaceVectorSearch) Search(ctx context.Context, _ *mcp.CallToolRequest, input WorkspaceVectorSearchInput) (*mcp.CallToolResult, WorkspaceVectorSearchOutput, error) {
	if s == nil || s.DB == nil || s.Embedder == nil {
		return nil, WorkspaceVectorSearchOutput{}, fmt.Errorf("vector search requires surreal client and embedder")
	}
	wsID := strings.TrimSpace(input.WorkspaceID)
	if wsID == "" {
		return nil, WorkspaceVectorSearchOutput{}, fmt.Errorf("workspaceId is required")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, WorkspaceVectorSearchOutput{}, fmt.Errorf("query is required")
	}

	topK := input.TopK
	if topK <= 0 {
		topK = 5
	}
	if topK > 50 {
		topK = 50
	}

	modelID, err := s.resolveModel(ctx, wsID, input.ModelID)
	if err != nil {
		return nil, WorkspaceVectorSearchOutput{}, err
	}

	// modelID := input.ModelID

	if input.ModelID != "" {
		if id, err := lookupVectorModelID(ctx, s.DB, wsID, input.ModelID); err == nil {
			modelID = id
		} else {
			return nil, WorkspaceVectorSearchOutput{}, err
		}
	}

	includeSet := normalizeFilters(input.FileFilter)
	includeList := make([]string, 0, len(includeSet))
	for rel := range includeSet {
		includeList = append(includeList, rel)
	}

	// embed the query with the same model as stored vectors
	qvec, err := s.embedQuery(ctx, modelID, query)
	if err != nil {
		return nil, WorkspaceVectorSearchOutput{}, err
	}

	// println(fmt.Sprintf("Vector: %v", qvec))

	// Single KNN query across workspace; Surreal returns cosine distance
	q := fmt.Sprintf(`
SELECT * FROM (
    SELECT
  content_sha,
  start,
  end,
  token_count,
  file,
  model,
  ws,
  vector::distance::knn() AS distance
FROM vector_chunk
WHERE
  vector <|%d,COSINE|> $qvec
)
WHERE ws = type::thing('workspace', $ws_id)
  AND model = type::thing('vector_model', $model_id)
  AND (array::len($include) = 0 OR file.relpath IN $include)
  AND distance != NONE
ORDER BY distance ASC
LIMIT %d;
`, topK, topK)

	type row struct {
		File       string  `json:"file"`
		Start      int     `json:"start"`
		End        int     `json:"end"`
		TokenCount int     `json:"token_count"`
		ContentSHA string  `json:"content_sha"`
		Distance   float64 `json:"distance"`
	}

	params := map[string]any{
		"ws_id":    wsID,
		"model_id": modelID,
		"qvec":     qvec,
		"include":  includeList,
	}

	queryResults, err := surrealdb.Query[[]row](ctx, s.DB.Db, q, params)
	if err != nil {
		return nil, WorkspaceVectorSearchOutput{}, fmt.Errorf("knn query: %w", err)
	}
	if len(*queryResults) == 0 {
		return nil, WorkspaceVectorSearchOutput{Matches: make([]WorkspaceVectorMatch, 0)}, nil
	}

	matches := make([]WorkspaceVectorMatch, len((*queryResults)[0].Result))
	for i, r := range (*queryResults)[0].Result {

		sim := 1.0 - r.Distance // cosine distance → similarity
		matches[i] = WorkspaceVectorMatch{
			Score:      sim,
			File:       r.File,
			Start:      r.Start,
			End:        r.End,
			TokenCount: r.TokenCount,
			ContentSHA: r.ContentSHA,
		}
	}
	return nil, WorkspaceVectorSearchOutput{Matches: matches}, nil
}

func (s *WorkspaceVectorSearch) resolveModel(ctx context.Context, wsID, override string) (string, error) {
	if override = strings.TrimSpace(override); override != "" {
		return override, nil
	}
	type row struct {
		ModelID string `json:"model_id"`
	}
	const q = `
SELECT meta::id(model) AS model_id
FROM vector_chunk
WHERE ws = type::thing('workspace', $ws_id)
GROUP BY model_id
LIMIT 1
`
	rows, err := surreal.Query[row](ctx, s.DB, q, map[string]any{"ws_id": wsID})
	if err != nil {
		return "", fmt.Errorf("resolve model: %w", err)
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].ModelID) == "" {
		return "", fmt.Errorf("no vector model found for workspace")
	}
	return rows[0].ModelID, nil
}

func (s *WorkspaceVectorSearch) embedQuery(ctx context.Context, modelID, query string) ([]float32, error) {
	if me, ok := any(s.Embedder).(modelAwareEmbedder); ok && modelID != "" {
		vecs, err := me.EmbedWithModel(ctx, modelID, []string{query})
		if err == nil && len(vecs) > 0 && len(vecs[0]) > 0 {
			return vecs[0], nil
		}
		// fall through to generic path on error/empty
	}
	vecs, err := s.Embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, fmt.Errorf("embedding returned empty vector")
	}
	return vecs[0], nil
}

func normalizeFilters(filters []string) map[string]struct{} {
	if len(filters) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(filters))
	for _, f := range filters {
		if trimmed := strings.TrimSpace(f); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}
