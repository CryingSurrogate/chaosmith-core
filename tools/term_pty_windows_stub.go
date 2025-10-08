//go:build !windows

package tools

import "fmt"

func startWindowsPTY(command string, args []string, cols, rows uint16) (*ptyHandle, error) {
	return nil, fmt.Errorf("windows PTY not available on this platform")
}
