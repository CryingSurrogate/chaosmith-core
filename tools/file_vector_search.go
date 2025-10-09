package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/embedder"
	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/surrealdb/surrealdb.go"
)

type FileVectorSearch struct {
	DB       *surreal.Client
	Embedder *embedder.Client
}

type FileVectorSearchInput struct {
	WorkspaceID string `json:"workspaceId" jsonschema:"workspace identifier"`
	RelPath     string `json:"relpath" jsonschema:"file path relative to workspace root"`
	Query       string `json:"query" jsonschema:"natural language query"`
	TopK        int    `json:"topK,omitempty" jsonschema:"number of matches to return (default 5, max 20)"`
	ModelID     string `json:"modelId,omitempty" jsonschema:"override vector model slug"`
}

type FileVectorSearchOutput struct {
	Matches []VectorMatch `json:"matches" jsonschema:"ranked vector matches"`
}

type VectorMatch struct {
	Score      float64 `json:"score" jsonschema:"cosine similarity score"`
	ContentSHA string  `json:"contentSha" jsonschema:"hash of the matched chunk"`
	Start      int     `json:"start" jsonschema:"chunk start byte offset"`
	End        int     `json:"end" jsonschema:"chunk end byte offset"`
	TokenCount int     `json:"tokenCount" jsonschema:"token count for the chunk"`
	Snippet    string  `json:"snippet" jsonschema:"text snippet of the chunk"`
}

func (s *FileVectorSearch) Search(ctx context.Context, _ *mcp.CallToolRequest, input FileVectorSearchInput) (*mcp.CallToolResult, FileVectorSearchOutput, error) {
	if s == nil || s.DB == nil || s.Embedder == nil {
		return nil, FileVectorSearchOutput{}, fmt.Errorf("vector search requires surreal client and embedder")
	}
	wsID := strings.TrimSpace(input.WorkspaceID)
	if wsID == "" {
		return nil, FileVectorSearchOutput{}, fmt.Errorf("workspaceId is required")
	}
	rel := strings.TrimSpace(input.RelPath)
	if rel == "" {
		return nil, FileVectorSearchOutput{}, fmt.Errorf("relpath is required")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, FileVectorSearchOutput{}, fmt.Errorf("query is required")
	}

	topK := input.TopK
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}

	limit := topK

	topK *= 1000

	wsPath, err := lookupWorkspacePath(ctx, s.DB, wsID)
	if err != nil {
		return nil, FileVectorSearchOutput{}, err
	}
	fileRecordID, err := lookupFileRecordID(ctx, s.DB, wsID, rel)
	if err != nil {
		return nil, FileVectorSearchOutput{}, err
	}

	println("File record ID: ", fileRecordID)

	modelID, err := s.resolveModel(ctx, fileRecordID, input.ModelID)
	if err != nil {
		return nil, FileVectorSearchOutput{}, err
	}

	if input.ModelID != "" {
		if id, err := lookupVectorModelID(ctx, s.DB, wsID, input.ModelID); err == nil {
			modelID = id
		} else {
			return nil, FileVectorSearchOutput{}, err
		}
	}

	// embed the query with the same model used for stored vectors
	qvec, err := s.embedQuery(ctx, modelID, query)
	if err != nil {
		return nil, FileVectorSearchOutput{}, err
	}

	// KNN directly in SurrealDB; returns cosine distance via vector::distance::knn()
	q := fmt.Sprintf(`
SELECT * FROM (
SELECT
  content_sha,
  start,
  end,
  token_count,
  file,
  model,
  vector::distance::knn() AS distance
FROM vector_chunk
WHERE
  vector <|%d,COSINE|> $qvec
)
WHERE file = type::thing('file', $file_id) AND model = type::thing('vector_model', $model_id)

ORDER BY distance ASC
LIMIT %d;
`, topK, limit)

	type row struct {
		ContentSHA string  `json:"content_sha"`
		Start      int     `json:"start"`
		End        int     `json:"end"`
		TokenCount int     `json:"token_count"`
		Distance   float64 `json:"distance"`
	}

	params := map[string]any{
		"file_id":  fileRecordID,
		"model_id": modelID,
		"qvec":     qvec,
	}

	queryResults, err := surrealdb.Query[[]row](ctx, s.DB.Db, q, params)
	if err != nil {
		return nil, FileVectorSearchOutput{}, fmt.Errorf("knn query: %w", err)
	}
	if len(*queryResults) == 0 {
		return nil, FileVectorSearchOutput{Matches: make([]VectorMatch, 0)}, nil
	}

	// println(fmt.Sprintf("FILE RESULTS: %v", (*queryResults)[0].Result))

	fileBytes, err := os.ReadFile(filepath.Join(wsPath, filepath.FromSlash(rel)))
	if err != nil {
		return nil, FileVectorSearchOutput{}, fmt.Errorf("read file for snippet: %w", err)
	}

	matches := make([]VectorMatch, len((*queryResults)[0].Result))
	for i, r := range (*queryResults)[0].Result {
		// Surreal returns cosine distance; convert to similarity in [0..1]
		sim := 1.0 - r.Distance
		matches[i] = VectorMatch{
			Score:      sim,
			ContentSHA: r.ContentSHA,
			Start:      r.Start,
			End:        r.End,
			TokenCount: r.TokenCount,
			Snippet:    sliceSnippet(fileBytes, r.Start, r.End),
		}
	}

	return nil, FileVectorSearchOutput{Matches: matches}, nil
}

func (s *FileVectorSearch) resolveModel(ctx context.Context, fileRecordID, override string) (string, error) {
	if override = strings.TrimSpace(override); override != "" {
		return override, nil
	}
	type row struct {
		ModelID string `json:"model_id"`
	}
	const q = `
SELECT meta::id(model) AS model_id
FROM vector_chunk
WHERE file = type::thing('file', $file_id)
GROUP BY model_id
LIMIT 1
`
	rows, err := surreal.Query[row](ctx, s.DB, q, map[string]any{"file_id": fileRecordID})
	if err != nil {
		return "", fmt.Errorf("resolve model: %w", err)
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].ModelID) == "" {
		return "", fmt.Errorf("no vector model found for file")
	}
	return rows[0].ModelID, nil
}

// model-aware embedding with graceful fallback
type modelAwareEmbedder interface {
	EmbedWithModel(ctx context.Context, model string, inputs []string) ([][]float32, error)
}

func (s *FileVectorSearch) embedQuery(ctx context.Context, modelID, query string) ([]float32, error) {
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

func lookupWorkspacePath(ctx context.Context, db *surreal.Client, wsID string) (string, error) {
	type row struct {
		Path string `json:"path"`
	}
	const q = `
SELECT path FROM workspace WHERE id = type::thing('workspace', $ws_id) LIMIT 1
`
	rows, err := surreal.Query[row](ctx, db, q, map[string]any{"ws_id": wsID})
	if err != nil {
		return "", fmt.Errorf("lookup workspace path: %w", err)
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].Path) == "" {
		return "", fmt.Errorf("workspace %s not found or missing path", wsID)
	}
	return strings.TrimSpace(rows[0].Path), nil
}

func lookupFileRecordID(ctx context.Context, db *surreal.Client, wsID, rel string) (string, error) {
	type row struct {
		FileID string `json:"file_id"`
	}
	const q = `
SELECT meta::id(id) AS file_id
FROM file
WHERE ws = type::thing('workspace', $ws_id) AND relpath = $rel
LIMIT 1
`
	rows, err := surreal.Query[row](ctx, db, q, map[string]any{"ws_id": wsID, "rel": rel})
	if err != nil {
		return "", fmt.Errorf("lookup file id: %w", err)
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].FileID) == "" {
		return "", fmt.Errorf("file %s not found in workspace %s", rel, wsID)
	}
	return rows[0].FileID, nil
}

func sliceSnippet(data []byte, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	if start >= len(data) || start >= end {
		return ""
	}
	window := data[start:end]
	text := string(window)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.TrimSpace(text)
	if len(text) > 512 {
		text = text[:512] + "…"
	}
	return text
}
