package surreal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	maxStatementsPerCall = 500
	// Conservative cap to avoid SurrealDB HTTP 413 (length limit exceeded).
	// Many servers cap request bodies around 1MB; we keep well below.
	maxBytesPerCall = 512 * 1024
)

// Client wraps the SurrealDB /sql endpoint for PCS/1.3-native usage.
type Client struct {
	baseURL string
	user    string
	pass    string
	ns      string
	db      string

	http *http.Client
}

// NewClient constructs a Surreal client. urlStr must include scheme and host.
func NewClient(urlStr, user, pass, ns, db string) (*Client, error) {
	if strings.TrimSpace(urlStr) == "" {
		return nil, fmt.Errorf("surreal url is required")
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid surreal url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported surreal scheme %q", u.Scheme)
	}
	return &Client{
		baseURL: strings.TrimRight(u.String(), "/"),
		user:    user,
		pass:    pass,
		ns:      ns,
		db:      db,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// Exec runs the provided statements in order, batching per Surreal guidance.
// Statements must not include the terminal semicolon; the client appends it.
func (c *Client) Exec(ctx context.Context, statements []string) error {
	if len(statements) == 0 {
		return nil
	}
	// Estimate buffer size and split by both count and byte size limits.
	var (
		group []string
		size  int
	)
	// Base preface adds a small constant; include it once per batch.
	base := len("USE NS `` DB ``;\n") + len(c.ns) + len(c.db)
	size = base

	flush := func() error {
		if len(group) == 0 {
			return nil
		}
		if err := c.execChunk(ctx, group); err != nil {
			return fmt.Errorf("surreal exec chunk failed: %w (first statement: %s)", err, truncateStatement(group[0]))
		}
		group = group[:0]
		size = base
		return nil
	}

	for _, s := range statements {
		stmt := strings.TrimSpace(s)
		if stmt == "" {
			continue
		}
		// Cost of this statement in bytes within the HTTP body (+semicolon + newline if needed)
		add := len(stmt) + 1 + 1
		if len(group) > 0 && (size+add > maxBytesPerCall) {
			if err := flush(); err != nil {
				return err
			}
		}
		group = append(group, stmt)
		size += add
		if len(group) >= maxStatementsPerCall {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func (c *Client) execChunk(ctx context.Context, stmts []string) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "USE NS %s DB %s;\n", quoteIdent(c.ns), quoteIdent(c.db))
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

	if strings.TrimSpace(os.Getenv("CS_DEBUG_SQL")) != "" {
		log.Printf("[SQL] batch:\n%s", buf.String())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sql", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return fmt.Errorf("build surreal request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("surreal request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("surreal http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded []sqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return fmt.Errorf("decode surreal response: %w", err)
	}
	for _, res := range decoded {
		if !strings.EqualFold(res.Status, "OK") {
			return fmt.Errorf("surreal error: %s %s", res.Code, res.Detail)
		}
	}
	return nil
}

type sqlResponse struct {
	Status      string          `json:"status"`
	Time        string          `json:"time"`
	Result      json.RawMessage `json:"result"`
	Detail      string          `json:"detail"`
	Description string          `json:"description"`
	Code        string          `json:"code"`
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
