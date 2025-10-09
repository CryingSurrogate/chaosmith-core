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

	server := mcp.NewServer(&mcp.Implementation{Name: "chaosmith-central", Version: "v0.2.0"}, nil)
	l1 := &tools.L1IndexerTools{Engine: indexEngine}
	nodereg := &tools.NodeRegister{DB: surrealClient}
	wsreg := &tools.WorkspaceRegister{DB: surrealClient}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index.workspace.scan",
		Description: "PCS/1.3-native L1 scan: enumerate workspace directories/files and commit to SurrealDB.",
	}, l1.Scan)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index.workspace.embed",
		Description: "PCS/1.3-native L1 embedding: call local embedding executor and store vector_chunk rows.",
	}, l1.Embed)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index.workspace.all",
		Description: "Run full L1 pipeline (scan + embed) with UDCS-compliant reporting.",
	}, l1.All)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "node.register",
		Description: "Upsert a node record with optional metadata so workspaces can target it",
	}, nodereg.Register)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace.register",
		Description: "Upsert a workspace bound to an existing node so scan/embed have a target.",
	}, wsreg.Register)

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
