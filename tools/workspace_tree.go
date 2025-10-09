package tools

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type WorkspaceTree struct {
	DB *surreal.Client
}

type WorkspaceTreeInput struct {
	WorkspaceID string `json:"workspaceId" jsonschema:"workspace identifier"`
}

type WorkspaceTreeOutput struct {
	WorkspaceID string           `json:"workspaceId" jsonschema:"workspace identifier"`
	Directories []DirectoryEntry `json:"directories" jsonschema:"all directories with parent references"`
	Files       []WorkspaceFile  `json:"files" jsonschema:"all files with directory references"`
}

type DirectoryEntry struct {
	RelPath string `json:"relpath" jsonschema:"path relative to workspace root"`
	Name    string `json:"name" jsonschema:"directory name"`
	Parent  string `json:"parent" jsonschema:"parent directory relpath"`
	SHA     string `json:"sha,omitempty" jsonschema:"directory content hash"`
}

type WorkspaceFile struct {
	RelPath   string    `json:"relpath" jsonschema:"path relative to workspace root"`
	Name      string    `json:"name" jsonschema:"file name"`
	Directory string    `json:"directory" jsonschema:"parent directory relpath"`
	Lang      string    `json:"lang,omitempty" jsonschema:"language hint"`
	Size      int64     `json:"size" jsonschema:"file size in bytes"`
	MTime     time.Time `json:"mtime" jsonschema:"modification time (UTC)"`
	SHA       string    `json:"sha" jsonschema:"content hash"`
}

func (t *WorkspaceTree) List(ctx context.Context, _ *mcp.CallToolRequest, input WorkspaceTreeInput) (*mcp.CallToolResult, WorkspaceTreeOutput, error) {
	if t == nil || t.DB == nil {
		return nil, WorkspaceTreeOutput{}, fmt.Errorf("surreal client not configured")
	}
	wsID := strings.TrimSpace(input.WorkspaceID)
	if wsID == "" {
		return nil, WorkspaceTreeOutput{}, fmt.Errorf("workspaceId is required")
	}

	type dirRow struct {
		RelPath string `json:"relpath"`
		SHA     string `json:"sha"`
	}
	type fileRow struct {
		RelPath string    `json:"relpath"`
		Lang    string    `json:"lang"`
		Size    int64     `json:"size"`
		MTime   time.Time `json:"mtime"`
		SHA     string    `json:"sha"`
	}

	const dirQuery = `
SELECT relpath, sha
FROM directory
WHERE ws = type::thing('workspace', $ws_id)
ORDER BY relpath ASC
`
	const fileQuery = `
SELECT relpath, lang, size, mtime, sha
FROM file
WHERE ws = type::thing('workspace', $ws_id)
ORDER BY relpath ASC
`

	vars := map[string]any{"ws_id": wsID}

	dirs, err := surreal.Query[dirRow](ctx, t.DB, dirQuery, vars)
	if err != nil {
		return nil, WorkspaceTreeOutput{}, fmt.Errorf("fetch directories: %w", err)
	}
	files, err := surreal.Query[fileRow](ctx, t.DB, fileQuery, vars)
	if err != nil {
		return nil, WorkspaceTreeOutput{}, fmt.Errorf("fetch files: %w", err)
	}

	dirEntries := make([]DirectoryEntry, 0, len(dirs)+1)
	dirEntries = append(dirEntries, DirectoryEntry{
		RelPath: "",
		Name:    "/",
		Parent:  "",
	})
	for _, d := range dirs {
		parent := parentRelPath(d.RelPath)
		dirEntries = append(dirEntries, DirectoryEntry{
			RelPath: d.RelPath,
			Name:    leafName(d.RelPath),
			Parent:  parent,
			SHA:     d.SHA,
		})
	}
	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].RelPath < dirEntries[j].RelPath
	})

	wsFiles := make([]WorkspaceFile, 0, len(files))
	for _, f := range files {
		parent := parentRelPath(f.RelPath)
		entry := WorkspaceFile{
			RelPath:   f.RelPath,
			Name:      leafName(f.RelPath),
			Directory: parent,
			Lang:      f.Lang,
			Size:      f.Size,
			MTime:     f.MTime,
			SHA:       f.SHA,
		}
		wsFiles = append(wsFiles, entry)
	}
	sort.Slice(wsFiles, func(i, j int) bool {
		return wsFiles[i].RelPath < wsFiles[j].RelPath
	})

	return nil, WorkspaceTreeOutput{
		WorkspaceID: wsID,
		Directories: dirEntries,
		Files:       wsFiles,
	}, nil
}

func parentRelPath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ""
	}
	parent := path.Dir(rel)
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}

func leafName(rel string) string {
	if rel == "" {
		return "/"
	}
	return path.Base(rel)
}
