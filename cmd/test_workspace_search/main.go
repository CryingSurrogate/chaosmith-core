package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/config"
	"github.com/CryingSurrogate/chaosmith-core/internal/embedder"
	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/CryingSurrogate/chaosmith-core/tools" // adjust import to where your tools live
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mustEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("missing env %s", k)
	}
	return v
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.SetFlags(0)

	cfgPathFlag := flag.String("config", "etc/centralmcp.toml", "path to chaosmith central config (TOML)")
	flag.Parse()

	configPath := resolveConfigPath(*cfgPathFlag)
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	surrealClient, err := surreal.NewClient(cfg.SurrealURL, cfg.SurrealUser, cfg.SurrealPass, cfg.SurrealNS, cfg.SurrealDB)
	if err != nil {
		log.Fatalf("surreal client: %v", err)
	}

	embedClient := embedder.New(cfg.EmbedURL, cfg.EmbedModel)

	s := &tools.WorkspaceVectorSearch{DB: surrealClient, Embedder: embedClient}

	// quick args (edit to taste)
	in := tools.WorkspaceVectorSearchInput{
		WorkspaceID: "chaoscore", // e.g. "chaoscore"
		Query:       "function",  // e.g. "cursor adapter bug"
		TopK:        5,
		ModelID:     "nomic-embed-text-v1-5", // e.g. "vector_model:nomic-embed-text-v1-5" or empty
		FileFilter:  []string{},              // e.g. "README.md api/router.go"
	}

	// bogus MCP request just to satisfy signature
	var req mcp.CallToolRequest
	_, out, err := s.Search(ctx, &req, in)
	if err != nil {
		log.Fatalf("search: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)

	s2 := &tools.FileVectorSearch{DB: surrealClient, Embedder: embedClient}

	// quick args (edit to taste)
	in2 := tools.FileVectorSearchInput{
		WorkspaceID: "chaoscore", // e.g. "chaoscore"
		Query:       "function",  // e.g. "cursor adapter bug"
		TopK:        15,
		ModelID:     "nomic-embed-text-v1-5", // e.g. "vector_model:nomic-embed-text-v1-5" or empty
		RelPath:     "docs/README_CHAOSMITH.md",
	}

	// bogus MCP request just to satisfy signature
	var req2 mcp.CallToolRequest
	_, out2, err := s2.Search(ctx, &req2, in2)
	if err != nil {
		log.Fatalf("search: %v", err)
	}

	enc2 := json.NewEncoder(os.Stdout)
	enc2.SetIndent("", "  ")
	_ = enc2.Encode(out2)
}

func resolveConfigPath(proposed string) string {
	if proposed == "" {
		return ""
	}
	if _, err := os.Stat(proposed); err == nil {
		return proposed
	} else if !os.IsNotExist(err) {
		log.Fatalf("config path %s: %v", proposed, err)
	}

	if envPath := os.Getenv("CHAOSMITH_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}
	if abs, err := filepath.Abs(proposed); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	// Allow running with config delivered entirely via env vars.
	return ""
}
