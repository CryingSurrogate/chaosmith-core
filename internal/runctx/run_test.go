package runctx

import (
	"testing"
	"time"
)

func TestGenerateRunIDDeterministic(t *testing.T) {
	ts := time.Date(2025, 7, 10, 12, 30, 0, 0, time.UTC)
	a := GenerateRunID("workspace-alpha", "index.scan", ts)
	b := GenerateRunID("workspace-alpha", "index.scan", ts)
	if a != b {
		t.Fatalf("expected deterministic run id, got %q and %q", a, b)
	}
	c := GenerateRunID("workspace-alpha", "index.embed", ts)
	if c == a {
		t.Fatalf("expected different step to yield different run id, got %q", c)
	}
}
