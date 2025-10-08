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

	stmts, err := ix.buildEmbedStatements(run, chunks)
	if err != nil {
		return &embedResult{}, err
	}
	if err := ix.surreal.Exec(ctx, stmts); err != nil {
		log.Printf("index.embed surreal exec failed (workspace=%s): %v", run.WorkspaceID, err)
		return &embedResult{}, fmt.Errorf("surreal exec (embed) workspace %s: %w", run.WorkspaceID, err)
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
		contentSHA := hashBytes(content)
		chunks = append(chunks, &embedChunk{
			RelPath:    rel,
			Text:       string(content),
			ContentSHA: contentSHA,
			Size:       info.Size(),
		})
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

func (ix *Indexer) buildEmbedStatements(run *runctx.Run, chunks []*embedChunk) ([]string, error) {
	wsThing := surrealThing("workspace", run.WorkspaceID)
	modelSlug := modelIdentifier(ix.cfg.EmbedModel)
	modelRecord := record("vector_model", modelSlug)
	modelThing := surrealThing("vector_model", modelSlug)
	family, version := splitModel(ix.cfg.EmbedModel)

	// Determine model native dim from first non-zero vector to avoid bogus 0 or mismatch.
	nativeDim := 0
	for _, ch := range chunks {
		if n := len(ch.Vector); n > 0 {
			nativeDim = n
			break
		}
	}
	if nativeDim == 0 {
		return nil, fmt.Errorf("no vectors available to determine native dim")
	}

	var stmts []string
	stmts = append(stmts, fmt.Sprintf("UPSERT %s CONTENT { id_slug: %s, family: %s, version: %s, native_dim: %d, notes: %s };",
		modelRecord,
		surrealStringLiteral(modelSlug),
		surrealStringLiteral(family),
		surrealStringLiteral(version),
		nativeDim,
		surrealStringLiteral("generated via chaosmith-core"),
	))

	for _, ch := range chunks {
		if len(ch.Vector) == 0 {
			return nil, fmt.Errorf("missing embedding for %s", ch.RelPath)
		}
		fileRecID := fileID(run.WorkspaceID, ch.RelPath)
		fileRecord := record("file", fileRecID)
		fileEndpoint := relationEndpoint(fileRecord)
		vecID := vectorChunkID(run.WorkspaceID, fileRecID, "file")
		vecRecord := record("vector_chunk", vecID)
		vecEndpoint := relationEndpoint(vecRecord)
		end := len(ch.Text)
		stmt := fmt.Sprintf("UPSERT %s SET ws = %s, file = %s, symbol = NONE, granularity = %s, start = 0, end = %d, token_count = NONE, content_sha = %s, model = %s, model_sha = %s, native_dim = %d, effective_dim = %d, transform_id = %s, vector = %s, ts = %s;",
			vecRecord,
			wsThing,
			surrealThing("file", fileRecID),
			surrealStringLiteral("file"),
			end,
			surrealStringLiteral(ch.ContentSHA),
			modelThing,
			surrealStringLiteral(ix.cfg.EmbedModelSHA),
			ch.NativeDim,
			ix.cfg.EffectiveDim,
			surrealStringLiteral(ix.cfg.TransformID),
			vectorToSurreal(ch.Vector),
			surrealDatetime(time.Now().UTC()),
		)
		stmts = append(stmts, stmt)
		stmts = append(stmts, fmt.Sprintf("RELATE %s->file_has_vector->%s;", fileEndpoint, vecEndpoint))
	}

	// Compute and upsert workspace centroid vector (simple mean) and relate.
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
		kind := surrealStringLiteral("centroid@file")
		wsVecRecord := record("workspace_vector", hexID("wsv", run.WorkspaceID, modelSlug, "centroid@file"))
		wsVecEndpoint := relationEndpoint(wsVecRecord)
		stmt := fmt.Sprintf("UPSERT %s CONTENT { ws: %s, kind: %s, model: %s, vector: %s, sample: %d, ts: %s };",
			wsVecRecord,
			wsThing,
			kind,
			modelThing,
			vectorToSurreal(centroid),
			sample,
			surrealDatetime(time.Now().UTC()),
		)
		stmts = append(stmts, stmt)
		stmts = append(stmts, fmt.Sprintf("RELATE %s->workspace_has_vector->%s;", relationEndpoint(record("workspace", run.WorkspaceID)), wsVecEndpoint))
	}
	return stmts, nil
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
