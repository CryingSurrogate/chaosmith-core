package indexer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/runctx"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
	"github.com/zeebo/blake3"
)

type scanResult struct {
	Artifacts []string
}

type dirMeta struct {
	RelPath string    `json:"relpath"`
	Hash    string    `json:"hash"`
	ModTime time.Time `json:"mtime"`
}

type fileMeta struct {
	RelPath string    `json:"relpath"`
	Size    int64     `json:"size"`
	MTime   time.Time `json:"mtime"`
	Hash    string    `json:"hash"`
	Lang    string    `json:"lang"`
}

func (ix *Indexer) performScan(ctx context.Context, run *runctx.Run) (*scanResult, error) {
	root := run.WorkspaceRoot
	wsID := run.WorkspaceID

	// Ensure the workspace record has current metadata without clearing its node relation.
	if err := ix.surreal.MergeRecord(ctx, "workspace", wsID, map[string]any{
		"path":        root,
		"vcs":         "",
		"rev":         "",
		"content_sha": "",
	}); err != nil {
		return &scanResult{}, fmt.Errorf("surreal merge workspace %s: %w", wsID, err)
	}

	var dirs []dirMeta
	var files []fileMeta

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() && shouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		rel := normalizeRelPath(root, path)
		if d.IsDir() {
			dHash := hashString(path)
			dirs = append(dirs, dirMeta{
				RelPath: rel,
				Hash:    dHash,
				ModTime: info.ModTime().UTC(),
			})
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil
		}
		hash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash file %s: %w", path, err)
		}
		files = append(files, fileMeta{
			RelPath: rel,
			Size:    info.Size(),
			MTime:   info.ModTime().UTC(),
			Hash:    hash,
			Lang:    detectLanguage(path),
		})
		return nil
	})
	if err != nil {
		return &scanResult{}, err
	}

	// Upsert directories and relations using SDK helpers
	for _, dir := range dirs {
		dirRecID := dirID(wsID, dir.RelPath)
		if err := ix.surreal.UpsertRecord(ctx, "directory", dirRecID, map[string]any{
			"ws":      surrealmodels.NewRecordID("workspace", wsID),
			"relpath": dir.RelPath,
			"sha":     dir.Hash,
		}); err != nil {
			return &scanResult{}, fmt.Errorf("upsert directory %s: %w", dir.RelPath, err)
		}
		if err := ix.surreal.Relate(ctx, "workspace", wsID, "ws_contains_dir", "directory", dirRecID, nil); err != nil {
			return &scanResult{}, fmt.Errorf("relate workspace->dir %s: %w", dir.RelPath, err)
		}
		if parent := parentDirRel(dir.RelPath); parent != "" || dir.RelPath != "" {
			parentRecID := dirID(wsID, parent)
			if err := ix.surreal.Relate(ctx, "directory", parentRecID, "dir_contains_dir", "directory", dirRecID, nil); err != nil {
				return &scanResult{}, fmt.Errorf("relate parent->dir %s: %w", dir.RelPath, err)
			}
		}
	}

	// Upsert files and relate to parent directory
	for _, file := range files {
		fileRecID := fileID(wsID, file.RelPath)
		if err := ix.surreal.UpsertRecord(ctx, "file", fileRecID, map[string]any{
			"ws":      surrealmodels.NewRecordID("workspace", wsID),
			"relpath": file.RelPath,
			"lang":    file.Lang,
			"size":    file.Size,
			"mtime":   file.MTime,
			"sha":     file.Hash,
		}); err != nil {
			return &scanResult{}, fmt.Errorf("upsert file %s: %w", file.RelPath, err)
		}
		dirRel := parentDirRel(file.RelPath)
		dirRecID := dirID(wsID, dirRel)
		if err := ix.surreal.Relate(ctx, "directory", dirRecID, "dir_contains_file", "file", fileRecID, nil); err != nil {
			return &scanResult{}, fmt.Errorf("relate dir->file %s: %w", file.RelPath, err)
		}
	}

	var artifacts []string
	filesArtifact, err := ix.writeNDJSON(run.ArtifactDir, "files.ndjson", files)
	if err != nil {
		return &scanResult{}, err
	}
	run.AddArtifact(filesArtifact)
	artifacts = append(artifacts, filesArtifact)

	dirsArtifact, err := ix.writeNDJSON(run.ArtifactDir, "dirs.ndjson", dirs)
	if err != nil {
		return &scanResult{}, err
	}
	run.AddArtifact(dirsArtifact)
	artifacts = append(artifacts, dirsArtifact)

	return &scanResult{Artifacts: artifacts}, nil
}

func shouldSkipDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".hg", ".svn", "node_modules", ".idea", ".vscode":
		return true
	default:
		return false
	}
}

func (ix *Indexer) writeNDJSON(dir, name string, data any) (string, error) {
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("write artifact %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	switch v := data.(type) {
	case []fileMeta:
		for _, row := range v {
			if err := enc.Encode(row); err != nil {
				return "", err
			}
		}
	case []dirMeta:
		for _, row := range v {
			if err := enc.Encode(row); err != nil {
				return "", err
			}
		}
	case []*embedChunk:
		for _, row := range v {
			if err := enc.Encode(row); err != nil {
				return "", err
			}
		}
	default:
		return "", fmt.Errorf("unsupported artifact type %T", data)
	}
	return path, nil
}

// buildScanStatements is replaced by direct SDK calls via surreal.Client

func parentDirRel(rel string) string {
	if rel == "" {
		return ""
	}
	parent := filepath.ToSlash(filepath.Dir(rel))
	if parent == "." {
		return ""
	}
	if parent == "/" {
		return ""
	}
	return strings.TrimPrefix(parent, "./")
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum), nil
}

func hashString(v string) string {
	hasher := blake3.New()
	hasher.Write([]byte(v))
	return hex.EncodeToString(hasher.Sum(nil))
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "text"
	}
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".sh", ".bash":
		return "shell"
	case ".ps1":
		return "powershell"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}
