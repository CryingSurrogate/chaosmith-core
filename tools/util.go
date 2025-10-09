package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
)

func clampLimit(requested int, max int) int {
	if requested <= 0 {
		return max
	}
	if requested > max {
		return max
	}
	return requested
}

func lookupVectorModelID(ctx context.Context, db *surreal.Client, wsID, candidate string) (string, error) {
	cand := strings.TrimSpace(candidate)
	if cand == "" {
		return "", fmt.Errorf("empty model candidate")
	}
	// If it's already a record id like "vector_model:abc123", keep it.
	if strings.Contains(cand, ":") && strings.HasPrefix(cand, "vector_model:") {
		return cand, nil
	}
	// Otherwise treat as slug/name, resolve to the model used in this workspace
	type row struct {
		ModelID string `json:"model_id"`
	}
	const q = `
SELECT meta::id(model) AS model_id
FROM vector_chunk
WHERE ws = type::thing('workspace', $ws_id)
  AND (model.slug = $cand OR model.name = $cand OR meta::id(model) = $cand)
GROUP BY model_id
LIMIT 1
`
	rows, err := surreal.Query[row](ctx, db, q, map[string]any{"ws_id": wsID, "cand": cand})
	if err != nil {
		return "", fmt.Errorf("resolve model candidate: %w", err)
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].ModelID) == "" {
		return "", fmt.Errorf("no model matching %q found in workspace %s", cand, wsID)
	}
	return rows[0].ModelID, nil
}
