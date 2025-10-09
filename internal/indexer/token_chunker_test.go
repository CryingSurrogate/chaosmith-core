package indexer

import (
	"strings"
	"testing"
)

func TestTokenChunkerSplitsByTokenLimit(t *testing.T) {
	chunker, err := newTokenChunker("tiktoken/cl100k_base")
	if err != nil {
		t.Fatalf("new token chunker: %v", err)
	}

	input := strings.Repeat("hello world ", 3000)
	segments, err := chunker.chunk(input)
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(segments) == 0 {
		t.Fatalf("expected segments")
	}

	var rebuilt strings.Builder
	prevEnd := 0
	for i, seg := range segments {
		if seg.TokenCount == 0 {
			t.Fatalf("segment %d has zero tokens", i)
		}
		if seg.TokenCount > maxTokensPerChunk {
			t.Fatalf("segment %d exceeds token limit: %d", i, seg.TokenCount)
		}
		if seg.Start != prevEnd {
			t.Fatalf("segment %d start mismatch: got %d want %d", i, seg.Start, prevEnd)
		}
		if seg.End <= seg.Start {
			t.Fatalf("segment %d end <= start", i)
		}
		rebuilt.WriteString(seg.Text)
		prevEnd = seg.End
	}
	if rebuilt.String() != input {
		t.Fatalf("rebuilt text mismatch")
	}
}
