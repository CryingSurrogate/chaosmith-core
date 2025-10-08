package indexer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/runctx"
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

	// Ensure the workspace record exists so relations don't dangle.
	{
		wsRecord := record("workspace", wsID)
		// Do not set node here; schema asserts node != NONE. Use workspace.register to establish node.
		stmt := fmt.Sprintf("UPSERT %s SET path = %s, vcs = '', rev = '', content_sha = '';",
			wsRecord,
			surrealStringLiteral(root),
		)
		if err := ix.surreal.Exec(ctx, []string{stmt}); err != nil {
			return &scanResult{}, fmt.Errorf("surreal upsert workspace %s: %w", wsID, err)
		}
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

	stmts := buildScanStatements(wsID, dirs, files)
	if err := ix.surreal.Exec(ctx, stmts); err != nil {
		log.Printf("index.scan surreal exec failed (workspace=%s): %v", wsID, err)
		return &scanResult{}, fmt.Errorf("surreal exec (scan) workspace %s: %w", wsID, err)
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

func buildScanStatements(workspaceID string, dirs []dirMeta, files []fileMeta) []string {
	wsRecord := record("workspace", workspaceID)
	wsThing := surrealThing("workspace", workspaceID)
	wsEndpoint := relationEndpoint(wsRecord)
	var stmts []string

	for _, dir := range dirs {
		dirRecord := record("directory", dirID(workspaceID, dir.RelPath))
		dirEndpoint := relationEndpoint(dirRecord)
		stmt := fmt.Sprintf("UPSERT %s SET ws = %s, relpath = %s, sha = %s;",
			dirRecord,
			wsThing,
			surrealStringLiteral(dir.RelPath),
			surrealStringLiteral(dir.Hash),
		)
		stmts = append(stmts, stmt)
		stmts = append(stmts, fmt.Sprintf("RELATE %s->ws_contains_dir->%s;", wsEndpoint, dirEndpoint))

		if parent := parentDirRel(dir.RelPath); parent != "" || dir.RelPath != "" {
			parentRecord := record("directory", dirID(workspaceID, parent))
			parentEndpoint := relationEndpoint(parentRecord)
			stmts = append(stmts, fmt.Sprintf("RELATE %s->dir_contains_dir->%s;", parentEndpoint, dirEndpoint))
		}
	}

	for _, file := range files {
		fileRecord := record("file", fileID(workspaceID, file.RelPath))
		fileEndpoint := relationEndpoint(fileRecord)
		stmt := fmt.Sprintf("UPSERT %s SET ws = %s, relpath = %s, lang = %s, size = %d, mtime = %s, sha = %s;",
			fileRecord,
			wsThing,
			surrealStringLiteral(file.RelPath),
			surrealStringLiteral(file.Lang),
			file.Size,
			surrealDatetime(file.MTime),
			surrealStringLiteral(file.Hash),
		)
		stmts = append(stmts, stmt)

		dirRel := parentDirRel(file.RelPath)
		dirRecord := record("directory", dirID(workspaceID, dirRel))
		dirEndpoint := relationEndpoint(dirRecord)
		stmts = append(stmts, fmt.Sprintf("RELATE %s->dir_contains_file->%s;", dirEndpoint, fileEndpoint))
	}

	return stmts
}

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
