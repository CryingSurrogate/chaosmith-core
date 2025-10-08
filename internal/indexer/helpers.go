package indexer

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeebo/blake3"
)

func normalizeRelPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	if rel == "." {
		return ""
	}
	rel = filepath.ToSlash(rel)
	return strings.TrimPrefix(rel, "./")
}

func fileID(workspaceID, relpath string) string {
	return hexID("file", workspaceID, relpath)
}

func dirID(workspaceID, relpath string) string {
	return hexID("dir", workspaceID, relpath)
}

func vectorChunkID(workspaceID, fileID string, granularity string) string {
	return hexID("vec", workspaceID, fileID, granularity)
}

func hexID(prefix string, parts ...string) string {
	builder := strings.Builder{}
	for i, p := range parts {
		if i > 0 {
			builder.WriteByte('|')
		}
		builder.WriteString(strings.ToLower(strings.TrimSpace(p)))
	}
	sum := blake3.Sum256([]byte(builder.String()))
	return prefix + "-" + hex.EncodeToString(sum[:10])
}

func record(table, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return table
	}
	return surrealThing(table, id)
}

func surrealStringLiteral(val string) string {
	val = strings.ReplaceAll(val, "\\", "\\\\")
	val = strings.ReplaceAll(val, "'", "\\'")
	return "'" + val + "'"
}

func surrealDatetime(t time.Time) string {
	// SurrealDB 2.2 expects type::datetime('...') to cast ISO8601 strings.
	return "type::datetime(" + surrealStringLiteral(t.UTC().Format(time.RFC3339Nano)) + ")"
}

func surrealThing(table, id string) string {
	table = strings.TrimSpace(table)
	id = strings.TrimSpace(id)
	return "type::thing(" + surrealStringLiteral(table) + ", " + surrealStringLiteral(id) + ")"
}

func relationEndpoint(record string) string {
	record = strings.TrimSpace(record)
	if record == "" {
		return ""
	}
	return "(" + record + ")"
}
