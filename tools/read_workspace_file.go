package tools

import (
    "context"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/CryingSurrogate/chaosmith-core/internal/surreal"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ReadWorkspaceFile struct {
    DB *surreal.Client
}

type ReadWorkspaceFileInput struct {
    WorkspaceID string `json:"workspaceId" jsonschema:"workspace identifier"`
    RelPath     string `json:"relPath" jsonschema:"file path relative to workspace root"`
    Start       int    `json:"start" jsonschema:"start character offset (0-based)"`
    End         int    `json:"end" jsonschema:"end character offset (exclusive)"`
    Hex         bool   `json:"hex,omitempty" jsonschema:"when true, read as hex-encoded bytes and count hex characters"`
}

type ReadWorkspaceFileOutput struct {
    RelPath   string `json:"relPath" jsonschema:"file path relative to workspace root"`
    Chunk     string `json:"chunk" jsonschema:"requested slice of the file contents"`
    Hex       bool   `json:"hex" jsonschema:"true if hex mode was used"`
    Truncated bool   `json:"truncated" jsonschema:"true if output was truncated for transport size"`
}

func (r *ReadWorkspaceFile) Read(ctx context.Context, _ *mcp.CallToolRequest, input ReadWorkspaceFileInput) (*mcp.CallToolResult, ReadWorkspaceFileOutput, error) {
    const maxChunkChars = 60 * 1024

    if r == nil || r.DB == nil {
        return nil, ReadWorkspaceFileOutput{RelPath: strings.TrimSpace(input.RelPath), Chunk: "", Hex: input.Hex, Truncated: false}, fmt.Errorf("surreal client not configured")
    }

    wsID := strings.TrimSpace(input.WorkspaceID)
    if wsID == "" {
        return nil, ReadWorkspaceFileOutput{RelPath: strings.TrimSpace(input.RelPath), Chunk: "", Hex: input.Hex, Truncated: false}, fmt.Errorf("workspaceId is required")
    }

    rel := strings.TrimSpace(input.RelPath)
    if rel == "" {
        return nil, ReadWorkspaceFileOutput{RelPath: rel, Chunk: "", Hex: input.Hex, Truncated: false}, fmt.Errorf("relPath is required")
    }

    if filepath.IsAbs(rel) {
        return nil, ReadWorkspaceFileOutput{RelPath: rel, Chunk: "", Hex: input.Hex, Truncated: false}, fmt.Errorf("path provided is not relative")
    }

    if _, err := lookupFileRecordID(ctx, r.DB, wsID, rel); err != nil {
        return nil, ReadWorkspaceFileOutput{RelPath: rel, Chunk: "", Hex: input.Hex, Truncated: false}, err
    }

    wsPath, err := lookupWorkspacePath(ctx, r.DB, wsID)
    if err != nil {
        return nil, ReadWorkspaceFileOutput{RelPath: rel, Chunk: "", Hex: input.Hex, Truncated: false}, err
    }

    full := filepath.Join(wsPath, filepath.FromSlash(rel))
    data, err := os.ReadFile(full)
    if err != nil {
        return nil, ReadWorkspaceFileOutput{RelPath: rel, Chunk: "", Hex: input.Hex, Truncated: false}, fmt.Errorf("read file: %w", err)
    }

    start := input.Start
    end := input.End
    if start < 0 {
        start = 0
    }
    if end < 0 {
        end = 0
    }
    if end < start {
        end = start
    }

    var chunk string
    var truncated bool

    if input.Hex {
        totalHexLen := len(data) * 2
        if start > totalHexLen {
            start = totalHexLen
        }
        if end > totalHexLen {
            end = totalHexLen
        }
        if end-start > maxChunkChars {
            end = start + maxChunkChars
            truncated = true
        }

        nibbles := end - start
        byteStart := start / 2
        byteEnd := (end + 1) / 2
        if byteStart > len(data) {
            byteStart = len(data)
        }
        if byteEnd > len(data) {
            byteEnd = len(data)
        }
        seg := data[byteStart:byteEnd]
        hexBuf := make([]byte, len(seg)*2)
        hex.Encode(hexBuf, seg)
        hexStr := string(hexBuf)
        if start%2 == 1 && len(hexStr) > 0 {
            hexStr = hexStr[1:]
        }
        if nibbles < len(hexStr) {
            hexStr = hexStr[:nibbles]
        }
        chunk = hexStr

        if end >= totalHexLen {
            chunk += "<|EOF|>"
        }
        if truncated {
            chunk += ". . .truncated"
        }
    } else {
        runes := []rune(string(data))
        if start > len(runes) {
            start = len(runes)
        }
        if end > len(runes) {
            end = len(runes)
        }
        if end-start > maxChunkChars {
            end = start + maxChunkChars
            truncated = true
        }
        chunk = string(runes[start:end])
        if end >= len(runes) {
            chunk += "<|EOF|>"
        }
        if truncated {
            chunk += ". . .truncated"
        }
    }

    out := ReadWorkspaceFileOutput{
        RelPath:   rel,
        Chunk:     chunk,
        Hex:       input.Hex,
        Truncated: truncated,
    }
    return nil, out, nil
}

