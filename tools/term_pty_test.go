package tools

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResolveSessionID(t *testing.T) {
	req := &mcp.CallToolRequest{Session: nil}
	if got := resolveSessionID(req, "override"); got != "override" {
		t.Fatalf("resolveSessionID override: got %q", got)
	}

	if got := resolveSessionID(req, ""); got != "" {
		t.Fatalf("resolveSessionID without override should be empty, got %q", got)
	}
}

func TestNormalizeAction(t *testing.T) {
	cases := []struct {
		name     string
		action   string
		hasSess  bool
		input    PTYInput
		expected string
	}{
		{"explicit", "write", true, PTYInput{}, "write"},
		{"noSessionOpen", "", false, PTYInput{}, "open"},
		{"dataWrite", "", true, PTYInput{Data: "ls"}, "write"},
		{"fallback", "", true, PTYInput{}, "read"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeAction(tc.action, tc.hasSess, tc.input); got != tc.expected {
				t.Fatalf("normalizeAction: got %q want %q", got, tc.expected)
			}
		})
	}
}

func TestNormalizedSize(t *testing.T) {
	a, b := normalizedSize(0, 0)
	if a == 0 || b == 0 {
		t.Fatalf("normalizedSize should fallback to defaults, got %d %d", a, b)
	}

	a, b = normalizedSize(120, 60)
	if a != 120 || b != 60 {
		t.Fatalf("normalizedSize override failed, got %d %d", a, b)
	}
}

func TestIsClosedPipe(t *testing.T) {
	cases := []struct {
		input error
		want  bool
	}{
		{errors.New("read |1: file already closed"), true},
		{errors.New("use of closed file"), true},
		{errors.New("file already closed"), true},
		{errors.New("random error"), false},
	}
	for _, tc := range cases {
		if got := isClosedPipe(tc.input); got != tc.want {
			t.Fatalf("isClosedPipe(%v) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestResolveCommand(t *testing.T) {
	if got := resolveCommand("/bin/zsh"); got != "/bin/zsh" {
		t.Fatalf("resolveCommand explicit: got %q", got)
	}

	if runtime.GOOS == "windows" {
		if got := resolveCommand(""); !strings.HasSuffix(strings.ToLower(got), "pwsh.exe") {
			t.Fatalf("resolveCommand windows fallback: got %q", got)
		}
	} else {
		if got := resolveCommand(""); !strings.HasPrefix(got, "/") {
			t.Fatalf("resolveCommand unix fallback missing absolute path: got %q", got)
		}
	}
}

func TestExecPTYRejectsMissingSession(t *testing.T) {
	out := PTYInput{Action: "read"}
	_, _, err := ExecPTY(context.Background(), nil, out)
	if err == nil || !strings.Contains(err.Error(), "session id") {
		t.Fatalf("ExecPTY should require session id, got err=%v", err)
	}
}

func TestStripANSI(t *testing.T) {
	input := "\x1b[31mERROR\x1b[0m\r\n\x1b]0;title\x07"
	want := "ERROR\r\n"
	if got := stripANSI(input); got != want {
		t.Fatalf("stripANSI: got %q want %q", got, want)
	}
}
