package indexer

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/runctx"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
	"github.com/zeebo/blake3"
)

const (
	maxEmbedFileBytes = 256 * 1024
	embedBatchSize    = 16
)

type embedResult struct {
	Artifacts []string
}

type embedChunk struct {
	RelPath    string    `json:"relpath"`
	Index      int       `json:"index"`
	Start      int       `json:"start"`
	End        int       `json:"end"`
	TokenCount int       `json:"token_count"`
	Text       string    `json:"-"`
	ContentSHA string    `json:"content_sha"`
	Size       int64     `json:"size"`
	Vector     []float32 `json:"vector"`
	NativeDim  int       `json:"native_dim"`
}

func (ix *Indexer) performEmbedding(ctx context.Context, run *runctx.Run) (*embedResult, error) {
	root := run.WorkspaceRoot

	chunks, err := ix.collectEmbedChunks(ctx, root)
	if err != nil {
		return &embedResult{}, err
	}
	if len(chunks) == 0 {
		return &embedResult{}, fmt.Errorf("no embeddable files discovered")
	}

	if err := ix.populateVectors(ctx, chunks); err != nil {
		return &embedResult{}, err
	}

	if err := ix.storeEmbeddings(ctx, run, chunks); err != nil {
		log.Printf("index.embed surreal ops failed (workspace=%s): %v", run.WorkspaceID, err)
		return &embedResult{}, fmt.Errorf("surreal ops (embed) workspace %s: %w", run.WorkspaceID, err)
	}

	artifact, err := ix.writeNDJSON(run.ArtifactDir, "vectors.ndjson", chunks)
	if err != nil {
		return &embedResult{}, err
	}
	run.AddArtifact(artifact)

	return &embedResult{Artifacts: []string{artifact}}, nil
}

func (ix *Indexer) collectEmbedChunks(ctx context.Context, root string) ([]*embedChunk, error) {
	var chunks []*embedChunk
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() == 0 || info.Size() > maxEmbedFileBytes {
			return nil
		}
		rel := normalizeRelPath(root, path)
		if rel == "" {
			rel = filepath.Base(path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isBinary(content) {
			return nil
		}
		segments, err := ix.chunker.chunk(string(content))
		if err != nil {
			return fmt.Errorf("chunk file %s: %w", rel, err)
		}
		for i, seg := range segments {
			chunkText := seg.Text
			chunks = append(chunks, &embedChunk{
				RelPath:    rel,
				Index:      i,
				Start:      seg.Start,
				End:        seg.End,
				TokenCount: seg.TokenCount,
				Text:       chunkText,
				ContentSHA: hashBytes([]byte(chunkText)),
				Size:       int64(len(chunkText)),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return chunks, nil
}

func (ix *Indexer) populateVectors(ctx context.Context, chunks []*embedChunk) error {
	for i := 0; i < len(chunks); i += embedBatchSize {
		j := i + embedBatchSize
		if j > len(chunks) {
			j = len(chunks)
		}
		batch := chunks[i:j]
		inputs := make([]string, len(batch))
		for k, ch := range batch {
			inputs[k] = ch.Text
		}
		vectors, err := ix.embed.Embed(ctx, inputs)
		if err != nil {
			return err
		}
		for k, vec := range vectors {
			if len(vec) == 0 {
				return fmt.Errorf("embedding returned empty vector for %s", batch[k].RelPath)
			}
			batch[k].Vector = vec
			batch[k].NativeDim = len(vec)
		}
	}
	return nil
}

func (ix *Indexer) storeEmbeddings(ctx context.Context, run *runctx.Run, chunks []*embedChunk) error {
	wsID := run.WorkspaceID
	modelSlug := modelIdentifier(ix.cfg.EmbedModel)
	family, version := splitModel(ix.cfg.EmbedModel)

	// Determine model native dim
	nativeDim := 0
	for _, ch := range chunks {
		if n := len(ch.Vector); n > 0 {
			nativeDim = n
			break
		}
	}
	if nativeDim == 0 {
		return fmt.Errorf("no vectors available to determine native dim")
	}

	// Upsert model metadata
	if err := ix.surreal.UpsertRecord(ctx, "vector_model", modelSlug, map[string]any{
		"id_slug":    modelSlug,
		"family":     family,
		"version":    version,
		"native_dim": nativeDim,
		"model_sha":  ix.cfg.EmbedModelSHA,
		"notes":      "generated via chaosmith-core",
	}); err != nil {
		return fmt.Errorf("upsert vector_model: %w", err)
	}

	// Upsert chunks and relate
	now := time.Now().UTC()
	for _, ch := range chunks {
		if len(ch.Vector) == 0 {
			return fmt.Errorf("missing embedding for %s chunk %d", ch.RelPath, ch.Index)
		}
		fileRecID := fileID(wsID, ch.RelPath)
		vecID := vectorChunkID(wsID, fileRecID, "chunk", ch.Index)
		if err := ix.surreal.UpsertRecord(ctx, "vector_chunk", vecID, map[string]any{
			"ws":            surrealmodels.NewRecordID("workspace", wsID),
			"file":          surrealmodels.NewRecordID("file", fileRecID),
			"symbol":        surrealmodels.None,
			"granularity":   "file_chunk",
			"chunk_index":   ch.Index,
			"start":         ch.Start,
			"end":           ch.End,
			"token_count":   ch.TokenCount,
			"content_sha":   ch.ContentSHA,
			"model":         surrealmodels.NewRecordID("vector_model", modelSlug),
			"model_sha":     ix.cfg.EmbedModelSHA,
			"native_dim":    ch.NativeDim,
			"effective_dim": ix.cfg.EffectiveDim,
			"transform_id":  ix.cfg.TransformID,
			"vector":        ch.Vector,
			"ts":            now,
		}); err != nil {
			return fmt.Errorf("upsert vector_chunk %s: %w", ch.RelPath, err)
		}
		if err := ix.surreal.Relate(ctx, "file", fileRecID, "file_has_vector", "vector_chunk", vecID, nil); err != nil {
			return fmt.Errorf("relate file->vector %s: %w", ch.RelPath, err)
		}
	}

	// Compute and upsert workspace centroid vector and relate
	centroid := make([]float32, nativeDim)
	sample := 0
	for _, ch := range chunks {
		if len(ch.Vector) != nativeDim {
			continue
		}
		for i := 0; i < nativeDim; i++ {
			centroid[i] += ch.Vector[i]
		}
		sample++
	}
	if sample > 0 {
		for i := 0; i < nativeDim; i++ {
			centroid[i] /= float32(sample)
		}
		wsVecID := hexID("wsv", wsID, modelSlug, "centroid@file")
		if err := ix.surreal.UpsertRecord(ctx, "workspace_vector", wsVecID, map[string]any{
			"ws":     surrealmodels.NewRecordID("workspace", wsID),
			"kind":   "centroid@file",
			"model":  surrealmodels.NewRecordID("vector_model", modelSlug),
			"vector": centroid,
			"sample": sample,
			"ts":     now,
		}); err != nil {
			return fmt.Errorf("upsert workspace_vector: %w", err)
		}
		if err := ix.surreal.Relate(ctx, "workspace", wsID, "workspace_has_vector", "workspace_vector", wsVecID, nil); err != nil {
			return fmt.Errorf("relate workspace->workspace_vector: %w", err)
		}
	}
	return nil
}

func isBinary(content []byte) bool {
	const sample = 1024
	n := len(content)
	if n > sample {
		n = sample
	}
	for i := 0; i < n; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

func hashBytes(b []byte) string {
	sum := blake3.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func vectorToSurreal(vec []float32) string {
	var sb strings.Builder
	sb.Grow(len(vec) * 8)
	sb.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%.8f", v))
	}
	sb.WriteByte(']')
	return sb.String()
}

func modelIdentifier(model string) string {
	slug := strings.ToLower(model)
	replacer := strings.NewReplacer(" ", "-", "/", "-", ":", "-", "@", "-", ".", "-", "_", "-")
	slug = replacer.Replace(slug)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
}

func splitModel(model string) (string, string) {
	parts := strings.Split(model, "-")
	if len(parts) < 2 {
		return modelIdentifier(model), "base"
	}
	family := parts[0]
	version := strings.Join(parts[1:], "-")
	return family, version
}
