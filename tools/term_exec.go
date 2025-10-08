package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Command string   `json:"command" jsonschema:"the command to execute"`
	Args    []string `json:"args,omitempty" jsonschema:"the command arguments in order (optional)"`
}

type Output struct {
	Stdout   string `json:"stdout" jsonschema:"captured standard output"`
	Stderr   string `json:"stderr,omitempty" jsonschema:"captured standard error"`
	ExitCode int    `json:"exitCode" jsonschema:"process exit code"`
	Error    string `json:"error,omitempty" jsonschema:"error message if execution failed"`
}

func ExecCommand(ctx context.Context, _ *mcp.CallToolRequest, input Input) (
	*mcp.CallToolResult, Output, error,
) {
	if strings.TrimSpace(input.Command) == "" {
		return nil, Output{}, fmt.Errorf("command is required")
	}

	cmd := exec.CommandContext(ctx, input.Command, input.Args...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	out := Output{
		Stdout: strings.TrimRight(stdout.String(), "\r\n"),
		Stderr: strings.TrimRight(stderr.String(), "\r\n"),
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		out.ExitCode = exitErr.ExitCode()
		out.Error = exitErr.Error()
		err = nil // surface the failure details via structured output
	} else if err != nil {
		out.Error = err.Error()
		out.ExitCode = -1
		return nil, out, nil
	} else {
		out.ExitCode = 0
	}

	return nil, out, nil
}
