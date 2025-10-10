package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/CryingSurrogate/chaosmith-core/internal/config"
	"github.com/CryingSurrogate/chaosmith-core/internal/embedder"
	"github.com/CryingSurrogate/chaosmith-core/internal/indexer"
	"github.com/CryingSurrogate/chaosmith-core/internal/surreal"
	"github.com/CryingSurrogate/chaosmith-core/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetFlags(0)

	cfgPathFlag := flag.String("config", "etc/centralmcp.toml", "path to chaosmith central config (TOML)")
	listenAddrFlag := flag.String("listen", ":9878", "HTTP listen address for MCP Streamable HTTP endpoint")
	enableStdio := flag.Bool("stdio", false, "also serve MCP over stdio (optional)")
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

	indexEngine, err := indexer.New(cfg, surrealClient)
	if err != nil {
		log.Fatalf("indexer init: %v", err)
	}
	embedClient := embedder.New(cfg.EmbedURL, cfg.EmbedModel)

	server := mcp.NewServer(&mcp.Implementation{Name: "chaosmith-central", Version: "v0.2.0"}, nil)
	l1 := &tools.L1IndexerTools{Engine: indexEngine}
	listNodes := &tools.ListNodes{DB: surrealClient}
	listWorkspaces := &tools.ListWorkspaces{DB: surrealClient}
	nodereg := &tools.NodeRegister{DB: surrealClient}
	fileVector := &tools.FileVectorSearch{DB: surrealClient, Embedder: embedClient}
	findFile := &tools.FindFile{DB: surrealClient}
	fileTextSearch := &tools.FileSearchText{DB: surrealClient}
	textSearch := &tools.WorkspaceSearchText{DB: surrealClient}
	tree := &tools.WorkspaceTree{DB: surrealClient}
	wsVector := &tools.WorkspaceVectorSearch{DB: surrealClient, Embedder: embedClient}
	wsreg := &tools.WorkspaceRegister{DB: surrealClient}
	reader := &tools.ReadWorkspaceFile{DB: surrealClient}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_workspace_scan",
		Description: "PCS/1.3-native L1 scan: enumerate workspace directories/files and commit to SurrealDB.",
	}, l1.Scan)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_workspace_embed",
		Description: "PCS/1.3-native L1 embedding: call local embedding executor and store vector_chunk rows.",
	}, l1.Embed)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_workspace_all",
		Description: "Run full L1 pipeline (scan + embed) with UDCS-compliant reporting.",
	}, l1.All)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "node_register",
		Description: "Upsert a node record with optional metadata so workspaces can target it",
	}, nodereg.Register)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "node_list",
		Description: "List all registered nodes with metadata",
	}, listNodes.List)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_list",
		Description: "List all registered workspaces",
	}, listWorkspaces.List)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_tree",
		Description: "Return directory and file tree for a workspace",
	}, tree.List)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_find_file",
		Description: "Find files in a workspace by exact/partial path",
	}, findFile.Search)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_search_text",
		Description: "Find exact text within workspace files",
	}, textSearch.Search)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "file_search_text",
		Description: "Find exact text within a specific workspace file",
	}, fileTextSearch.Search)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "file_vector_search",
		Description: "Vector similarity search within a workspace file",
	}, fileVector.Search)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_vector_search",
		Description: "Vector similarity search across a workspace",
	}, wsVector.Search)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_register",
		Description: "Upsert a workspace bound to an existing node so scan/embed have a target.",
	}, wsreg.Register)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_workspace_file",
		Description: "Read a file span from a workspace with optional hex encoding.",
	}, reader.Read)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "term_exec",
		Description: "Execute a command in non-interactive terminal",
	}, tools.ExecCommand)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "term_pty",
		Description: "Manage an interactive pseudo-terminal session scoped to the MCP session",
	}, tools.ExecPTY)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: false})

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", handler.ServeHTTP)

	httpSrv := &http.Server{
		Addr:              *listenAddrFlag,
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	go func() {
		log.Printf("chaosmith-central: StreamableHTTP listening on %s/mcp", *listenAddrFlag)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	if *enableStdio {
		go func() {
			if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
				log.Fatalf("stdio server: %v", err)
			}
		}()
	}

	<-ctx.Done()
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
