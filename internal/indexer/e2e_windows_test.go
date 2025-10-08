package indexer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/config"
	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
)

// TestEndToEnd_Workspace_Bloodseeker simulates registering, scanning, and embedding
// a Windows workspace at C:\Users\Adminz\_SOW\Bloodseeker. It logs SurrealQL
// queries and embed requests to the console and asserts the run succeeds.
func TestEndToEnd_Workspace_Bloodseeker(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only e2e test")
	}

	// Prepare fake Surreal /sql endpoint that logs incoming SQL payloads.
	surrealLogs := make([]string, 0, 8)
	surrealSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/sql") {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		t.Logf("/sql request:\n%s", string(body))
		surrealLogs = append(surrealLogs, string(body))
		w.Header().Set("Content-Type", "application/json")
		// Minimal Surreal-like response indicating OK.
		_, _ = w.Write([]byte(`[{"status":"OK"}]`))
	}))
	defer surrealSrv.Close()

	// Prepare fake embed endpoint that logs requests and returns constant vectors.
	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		t.Logf("/embeddings request: model=%s, inputs=%d", req.Model, len(req.Input))
		type row struct {
			Embedding []float32 `json:"embedding"`
		}
		resp := struct {
			Data []row `json:"data"`
		}{Data: make([]row, len(req.Input))}
		// 4-dim toy vectors
		for i := range resp.Data {
			resp.Data[i].Embedding = []float32{0.1, 0.2, 0.3, 0.4}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer embedSrv.Close()

	// Create the test workspace tree.
	workspaceRoot := `C:\Users\Adminz\_SOW\Bloodseeker`
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	// Add small files for scan + embed
	mustWrite(t, filepath.Join(workspaceRoot, "README.md"), "# Bloodseeker\nTest corpus\n")
	mustWrite(t, filepath.Join(workspaceRoot, "src", "main.go"), "package main\nfunc main(){}\n")

	// Set up config + clients.
	cfg := &config.Config{
		SurrealURL:    surrealSrv.URL,
		SurrealUser:   "",
		SurrealPass:   "",
		SurrealNS:     "chaosmith",
		SurrealDB:     "wims",
		EmbedKind:     "openai",
		EmbedURL:      embedSrv.URL,
		EmbedModel:    "unit-test-model",
		EmbedModelSHA: "sha256-unit",
		EffectiveDim:  4,
		TransformID:   "pca-unit@deadbeef",
		TokenizerID:   "tiktoken/cl100k_base",
		ArtifactRoot:  t.TempDir(),
	}
	client, err := surreal.NewClient(cfg.SurrealURL, cfg.SurrealUser, cfg.SurrealPass, cfg.SurrealNS, cfg.SurrealDB)
	if err != nil {
		t.Fatalf("surreal client: %v", err)
	}
	ix, err := New(cfg, client)
	if err != nil {
		t.Fatalf("indexer init: %v", err)
	}

	// Run the full pipeline.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	report, err := ix.All(ctx, WorkspaceRequest{
		WorkspaceRoot: workspaceRoot,
		WorkspaceID:   "chaoscore",
	})
	if err != nil {
		t.Fatalf("index all error: %v", err)
	}
	if report == nil || report.Acceptance != "pass" {
		t.Fatalf("unexpected run report: %+v", report)
	}
	t.Logf("Run OK: id=%s step=%s artifacts=%v", report.RunID, report.Step, report.ArtifactPaths)

	if len(surrealLogs) == 0 {
		t.Fatalf("expected at least one /sql call")
	}
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}
