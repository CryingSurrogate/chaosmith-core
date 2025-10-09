package indexer

import (
	"fmt"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

const maxTokensPerChunk = 768

type tokenChunk struct {
	Text       string
	Start      int
	End        int
	TokenCount int
}

type tokenChunker struct {
	enc *tiktoken.Tiktoken
}

func newTokenChunker(tokenizerID string) (*tokenChunker, error) {
	id := strings.TrimSpace(tokenizerID)
	if id == "" {
		return nil, fmt.Errorf("tokenizer id is required")
	}
	id = strings.TrimPrefix(id, "tiktoken/")

	enc, err := tiktoken.GetEncoding(id)
	if err != nil {
		enc, err = tiktoken.EncodingForModel(id)
		if err != nil {
			return nil, fmt.Errorf("load tokenizer %s: %w", tokenizerID, err)
		}
	}
	return &tokenChunker{enc: enc}, nil
}

func (c *tokenChunker) chunk(text string) ([]tokenChunk, error) {
	if c == nil || c.enc == nil {
		return nil, fmt.Errorf("token chunker not initialised")
	}
	tokens := c.enc.Encode(text, nil, nil)
	if len(tokens) == 0 {
		return nil, nil
	}

	chunks := make([]tokenChunk, 0, (len(tokens)+maxTokensPerChunk-1)/maxTokensPerChunk)
	byteCursor := 0
	for start := 0; start < len(tokens); start += maxTokensPerChunk {
		end := start + maxTokensPerChunk
		if end > len(tokens) {
			end = len(tokens)
		}

		chunkTokens := tokens[start:end]
		chunkText := c.enc.Decode(chunkTokens)
		if len(chunkText) == 0 {
			continue
		}

		if byteCursor+len(chunkText) > len(text) || text[byteCursor:byteCursor+len(chunkText)] != chunkText {
			idx := strings.Index(text[byteCursor:], chunkText)
			if idx == -1 {
				return nil, fmt.Errorf("token chunk alignment failed at byte %d", byteCursor)
			}
			byteCursor += idx
		}

		startPos := byteCursor
		endPos := byteCursor + len(chunkText)
		chunks = append(chunks, tokenChunk{
			Text:       text[startPos:endPos],
			Start:      startPos,
			End:        endPos,
			TokenCount: len(chunkTokens),
		})
		byteCursor = endPos
	}

	return chunks, nil
}
