package surreal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientExecBatchesStatements(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		requests = append(requests, string(body))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]sqlResponse{{Status: "OK"}})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", "", "chaos", "smith")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	var statements []string
	for i := 0; i < maxStatementsPerCall+3; i++ {
		statements = append(statements, fmt.Sprintf("UPDATE test SET idx = %d", i))
	}

	if err := client.Exec(context.Background(), statements); err != nil {
		t.Fatalf("exec: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}

	for _, req := range requests {
		if !strings.Contains(req, "USE NS `chaos` DB `smith`;") {
			t.Fatalf("request missing namespace/db prefix: %s", req)
		}
		if !strings.HasSuffix(strings.TrimSpace(req), ";") {
			t.Fatalf("request missing trailing semicolon: %s", req)
		}
	}
}
