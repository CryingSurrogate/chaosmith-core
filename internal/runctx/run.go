package runctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeebo/blake3"
)

// Run captures a single orchestrated run of an index step.
type Run struct {
	RunID         string
	WorkspaceID   string
	WorkspaceRoot string
	Step          string
	Started       time.Time
	ArtifactDir   string

	artifacts []string
}

// New constructs a Run, creating the artifact directory under artifactRoot/runID.
// If runID is empty a deterministic id derived from workspace, step, and start time is generated.
func New(artifactRoot, runID, workspaceID, workspaceRoot, step string, started time.Time) (*Run, error) {
	if started.IsZero() {
		started = time.Now().UTC()
	}
	if runID == "" {
		runID = GenerateRunID(workspaceID, step, started)
	}
	if step == "" {
		return nil, fmt.Errorf("step is required")
	}

	artifactDir := filepath.Join(artifactRoot, runID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir %s: %w", artifactDir, err)
	}

	return &Run{
		RunID:         runID,
		WorkspaceID:   workspaceID,
		WorkspaceRoot: workspaceRoot,
		Step:          step,
		Started:       started,
		ArtifactDir:   artifactDir,
	}, nil
}

// GenerateRunID creates RUN-YYYYMMDD-<8 hex> identifiers per PCS/1.3-native guidance.
func GenerateRunID(workspaceID, step string, started time.Time) string {
	if started.IsZero() {
		started = time.Now().UTC()
	}
	started = started.UTC()
	input := []byte(strings.Join([]string{
		workspaceID,
		step,
		started.Format(time.RFC3339Nano),
	}, "|"))
	sum := blake3.Sum256(input)
	return fmt.Sprintf("RUN-%s-%x", started.Format("20060102"), sum[:4])
}

// AddArtifact records a path stored inside the run artifact tree.
func (r *Run) AddArtifact(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	r.artifacts = append(r.artifacts, path)
}

// Artifacts returns all artifacts registered with the run.
func (r *Run) Artifacts() []string {
	out := make([]string, len(r.artifacts))
	copy(out, r.artifacts)
	return out
}
