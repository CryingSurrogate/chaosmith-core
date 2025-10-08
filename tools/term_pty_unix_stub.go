//go:build windows

package tools

import "fmt"

func startUnixPTY(command string, args []string, cols, rows uint16) (*ptyHandle, error) {
	return nil, fmt.Errorf("unix PTY not available on this platform")
}
