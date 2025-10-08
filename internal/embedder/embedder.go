package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client sends embedding requests to local executors per PCS/1.3-native.
type Client struct {
	Endpoint string
	Model    string

	http *http.Client
}

// New returns a configured embedding client.
func New(endpoint, model string) *Client {
	return &Client{
		Endpoint: strings.TrimRight(endpoint, "/"),
		Model:    model,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Embed returns embeddings for each input string in order.
func (c *Client) Embed(ctx context.Context, input []string) ([][]float32, error) {
	if len(input) == 0 {
		return nil, nil
	}
	payload := struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{
		Model: c.Model,
		Input: input,
	}
	body, _ := json.Marshal(payload)

	if strings.TrimSpace(os.Getenv("CS_DEBUG_EMBED")) != "" {
		log.Printf("[EMBED] POST %s model=%s inputs=%d", c.Endpoint, c.Model, len(input))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("embed http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var decoded struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	if len(decoded.Data) != len(input) {
		return nil, fmt.Errorf("embed response count mismatch: expected %d got %d", len(input), len(decoded.Data))
	}
	out := make([][]float32, len(decoded.Data))
	for i, row := range decoded.Data {
		out[i] = row.Embedding
	}
	return out, nil
}
