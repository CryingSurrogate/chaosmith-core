package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/config"
	"github.com/CryingSurrogate/chaosmith-core/internal/embedder"
	"github.com/CryingSurrogate/chaosmith-core/internal/runctx"
	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
)

// Step identifiers used for run IDs and reporting.
const (
	StepScan   = "index.scan"
	StepEmbed  = "index.embed"
	StepAll    = "index.all"
	StepSymbol = "index.symbols"
)

// WorkspaceRequest carries input parameters from MCP tools.
type WorkspaceRequest struct {
	WorkspaceRoot string `json:"workspaceRoot"`
	WorkspaceID   string `json:"workspaceId"`
	RunID         string `json:"runId,omitempty"`
	NodeID        string `json:"nodeId,omitempty"`
}

// RunReport summarises execution for the orchestrator per PCS/INST/1.0 style guide.
type RunReport struct {
	RunID         string    `json:"run_id"`
	Step          string    `json:"step"`
	Started       time.Time `json:"started"`
	Finished      time.Time `json:"finished"`
	Acceptance    string    `json:"acceptance"` // "pass" or "fail"
	ArtifactPaths []string  `json:"artifact_paths"`
	Risks         []string  `json:"risks,omitempty"`
	Notes         []string  `json:"notes,omitempty"`
}

// Indexer orchestrates workspace scanning and embedding.
type Indexer struct {
	cfg     *config.Config
	surreal *surreal.Client
	embed   *embedder.Client
	chunker *tokenChunker
}

// New builds an Indexer from configuration and Surreal client.
func New(cfg *config.Config, surrealClient *surreal.Client) (*Indexer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if surrealClient == nil {
		return nil, fmt.Errorf("surreal client is required")
	}
	embedClient := embedder.New(cfg.EmbedURL, cfg.EmbedModel)
	chunker, err := newTokenChunker(cfg.TokenizerID)
	if err != nil {
		return nil, fmt.Errorf("tokenizer init: %w", err)
	}
	return &Indexer{
		cfg:     cfg,
		surreal: surrealClient,
		embed:   embedClient,
		chunker: chunker,
	}, nil
}

// Scan indexes directories and files into SurrealDB.
func (ix *Indexer) Scan(ctx context.Context, req WorkspaceRequest) (*RunReport, error) {
	if err := validateWorkspaceRequest(req); err != nil {
		return nil, err
	}
	run, err := runctx.New(ix.cfg.ArtifactRoot, req.RunID, req.WorkspaceID, req.WorkspaceRoot, StepScan, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	report := &RunReport{
		RunID:   run.RunID,
		Step:    StepScan,
		Started: run.Started,
		Risks:   []string{},
		Notes:   []string{},
	}

	scanRes, err := ix.performScan(ctx, run)
	if err != nil {
		report.Acceptance = "fail"
		report.Risks = append(report.Risks, err.Error())
		return report, err
	}

	report.Finished = time.Now().UTC()
	report.Acceptance = "pass"
	report.ArtifactPaths = append(report.ArtifactPaths, scanRes.Artifacts...)
	return report, nil
}

// Embed produces vectors for the workspace and stores them.
func (ix *Indexer) Embed(ctx context.Context, req WorkspaceRequest) (*RunReport, error) {
	if err := validateWorkspaceRequest(req); err != nil {
		return nil, err
	}
	run, err := runctx.New(ix.cfg.ArtifactRoot, req.RunID, req.WorkspaceID, req.WorkspaceRoot, StepEmbed, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	report := &RunReport{
		RunID:   run.RunID,
		Step:    StepEmbed,
		Started: run.Started,
		Risks:   []string{},
		Notes:   []string{},
	}

	embedRes, err := ix.performEmbedding(ctx, run)
	if err != nil {
		report.Acceptance = "fail"
		report.Risks = append(report.Risks, err.Error())
		return report, err
	}

	report.Finished = time.Now().UTC()
	report.Acceptance = "pass"
	report.ArtifactPaths = append(report.ArtifactPaths, embedRes.Artifacts...)
	return report, nil
}

// All runs scan then embed sequentially.
func (ix *Indexer) All(ctx context.Context, req WorkspaceRequest) (*RunReport, error) {
	if err := validateWorkspaceRequest(req); err != nil {
		return nil, err
	}
	run, err := runctx.New(ix.cfg.ArtifactRoot, req.RunID, req.WorkspaceID, req.WorkspaceRoot, StepAll, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	report := &RunReport{
		RunID:   run.RunID,
		Step:    StepAll,
		Started: run.Started,
		Risks:   []string{},
		Notes:   []string{},
	}

	scanRes, err := ix.performScan(ctx, run)
	if err != nil {
		report.Acceptance = "fail"
		report.Risks = append(report.Risks, fmt.Sprintf("scan failed: %s", err))
		report.ArtifactPaths = append(report.ArtifactPaths, scanRes.Artifacts...)
		return report, err
	}
	embedRes, err := ix.performEmbedding(ctx, run)
	if err != nil {
		report.Acceptance = "fail"
		report.Risks = append(report.Risks, fmt.Sprintf("embedding failed: %s", err))
		report.ArtifactPaths = append(report.ArtifactPaths, append(scanRes.Artifacts, embedRes.Artifacts...)...)
		return report, err
	}

	report.Finished = time.Now().UTC()
	report.Acceptance = "pass"
	report.ArtifactPaths = append(report.ArtifactPaths, append(scanRes.Artifacts, embedRes.Artifacts...)...)
	return report, nil
}

func validateWorkspaceRequest(req WorkspaceRequest) error {
	if strings.TrimSpace(req.WorkspaceRoot) == "" {
		return fmt.Errorf("workspaceRoot is required")
	}
	if strings.TrimSpace(req.WorkspaceID) == "" {
		return fmt.Errorf("workspaceId is required")
	}
	info, err := os.Stat(req.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("workspace root access: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root %s is not a directory", req.WorkspaceRoot)
	}
	abs, err := filepath.Abs(req.WorkspaceRoot)
	if err == nil {
		req.WorkspaceRoot = abs
	}
	return nil
}
