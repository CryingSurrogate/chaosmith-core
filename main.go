package main

import (
	"log"
	"net/http"

	"github.com/CryingSurrogate/chaosmith-core/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// var evStore mcp.EventStore

func main() {
	// evStore := mcp.NewMemoryEventStore(&mcp.MemoryEventStoreOptions{})

	// Create a server and register available tools.
	server := mcp.NewServer(&mcp.Implementation{Name: "chaosmith-central", Version: "v0.1.1"}, nil)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: false})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "term_exec",
		Description: "Execute a command in non-interactive terminal",
	}, tools.ExecCommand)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "term_pty",
		Description: "Manage an interactive pseudo-terminal session scoped to the MCP session",
	}, tools.ExecPTY)

	http.HandleFunc("/mcp", handler.ServeHTTP)
	log.Fatal(http.ListenAndServe(":9878", nil))
}
