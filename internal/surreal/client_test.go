package surreal

import (
    "context"
    "fmt"
    "strings"
    "testing"

    surrealdb "github.com/surrealdb/surrealdb.go"
)

type fakeRunner struct{ batches []string }

func (f *fakeRunner) Run(_ context.Context, _ *surrealdb.DB, sql string, _ map[string]any) error {
    f.batches = append(f.batches, sql)
    return nil
}

func TestClientExecJoinsStatements(t *testing.T) {
    f := &fakeRunner{}
    client := &Client{ns: "chaos", dbName: "smith", runner: f}

    var statements []string
    for i := 0; i < 3; i++ {
        statements = append(statements, fmt.Sprintf("UPDATE test SET idx = %d", i))
    }

    if err := client.Exec(context.Background(), statements); err != nil {
        t.Fatalf("exec: %v", err)
    }

    if len(f.batches) != 1 {
        t.Fatalf("expected 1 batch, got %d", len(f.batches))
    }
    b := f.batches[0]
    if !strings.Contains(b, "USE NS `chaos` DB `smith`;") {
        t.Fatalf("batch missing namespace/db prefix: %s", b)
    }
    if !strings.HasSuffix(strings.TrimSpace(b), ";") {
        t.Fatalf("batch missing trailing semicolon: %s", b)
    }
}
