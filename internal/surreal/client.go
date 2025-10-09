package surreal

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	surrealdb "github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

// No HTTP batching/byte limits are needed when using the SDK over WebSocket.

// queryRunner abstracts execution for testability.
type queryRunner interface {
	Run(ctx context.Context, db *surrealdb.DB, sql string, vars map[string]any) error
}

type sdkRunner struct{}

func (sdkRunner) Run(ctx context.Context, db *surrealdb.DB, sql string, vars map[string]any) error {
	_, err := surrealdb.Query[[]any](ctx, db, sql, vars)
	return err
}

// Client wraps the SurrealDB Go SDK for PCS/1.3-native usage.
type Client struct {
	ns     string
	dbName string

	db     *surrealdb.DB
	runner queryRunner
}

// NewClient constructs a Surreal client using the official SDK.
// urlStr may be http/https/ws/wss. It will be normalized to ws(s)://.../rpc for the SDK.
func NewClient(urlStr, user, pass, ns, db string) (*Client, error) {
	if strings.TrimSpace(urlStr) == "" {
		return nil, fmt.Errorf("surreal url is required")
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid surreal url: %w", err)
	}
	// Prefer WebSocket engine. Map http->ws and https->wss.
	switch strings.ToLower(u.Scheme) {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	// Connect SDK client using the endpoint URL (supports ws/wss directly)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sdk, err := surrealdb.FromEndpointURLString(ctx, u.Scheme+"://"+u.Host+u.Path)
	if err != nil {
		return nil, fmt.Errorf("connect surreal sdk: %w", err)
	}

	// Authenticate if credentials provided
	if strings.TrimSpace(user) != "" || strings.TrimSpace(pass) != "" {
		if _, err := sdk.SignIn(ctx, surrealdb.Auth{Username: user, Password: pass}); err != nil {
			return nil, fmt.Errorf("surreal signin: %w", err)
		}
	}

	// Select namespace and database
	if err := sdk.Use(ctx, ns, db); err != nil {
		return nil, fmt.Errorf("surreal use ns/db: %w", err)
	}

	return &Client{
		ns:     ns,
		dbName: db,
		db:     sdk,
		runner: sdkRunner{},
	}, nil
}

// Exec runs the provided statements in a single multi-statement query.
// Statements must not include the terminal semicolon; the client appends it.
func (c *Client) Exec(ctx context.Context, statements []string) error {
	if len(statements) == 0 {
		return nil
	}
	return c.execChunk(ctx, statements)
}

func (c *Client) execChunk(ctx context.Context, stmts []string) error {
	var buf bytes.Buffer
	// Keep explicit USE for clarity and parity with previous behavior; harmless with SDK.
	fmt.Fprintf(&buf, "USE NS %s DB %s;\n", quoteIdent(c.ns), quoteIdent(c.dbName))
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		buf.WriteString(stmt)
		if !strings.HasSuffix(stmt, ";") {
			buf.WriteString(";")
		}
		buf.WriteByte('\n')
	}

	log.Printf("[SQL] batch:\n%s", buf.String())

	// Execute via SDK. We ignore results and rely on errors from the driver.
	if err := c.runner.Run(ctx, c.db, buf.String(), nil); err != nil {
		return fmt.Errorf("surreal query failed: %w", err)
	}
	return nil
}

func quoteIdent(id string) string {
	if strings.HasPrefix(id, "`") && strings.HasSuffix(id, "`") {
		return id
	}
	return "`" + strings.ReplaceAll(id, "`", "``") + "`"
}

func truncateStatement(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	if len(stmt) <= 160 {
		return stmt
	}
	return stmt[:157] + "..."
}

// UpsertRecord upserts a specific record by table and ID with the provided content.
func (c *Client) UpsertRecord(ctx context.Context, table, id string, content map[string]any) error {
	_, err := surrealdb.Upsert[map[string]any](ctx, c.db, models.NewRecordID(table, id), content)
	return err
}

// MergeRecord merges the provided content into an existing record without overwriting unspecified fields.
func (c *Client) MergeRecord(ctx context.Context, table, id string, content map[string]any) error {
	if len(content) == 0 {
		return nil
	}
	_, err := surrealdb.Merge[map[string]any](ctx, c.db, models.NewRecordID(table, id), content)
	return err
}

// Relate creates a relation from in -> relation -> out with optional data.
func (c *Client) Relate(ctx context.Context, inTable, inID, relation, outTable, outID string, data map[string]any) error {
	_, err := surrealdb.Relate[any](ctx, c.db, &surrealdb.Relationship{
		In:       models.NewRecordID(inTable, inID),
		Out:      models.NewRecordID(outTable, outID),
		Relation: models.Table(relation),
		Data:     data,
	})
	return err
}
