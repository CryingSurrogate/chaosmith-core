//go:build !windows

package tools

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startUnixPTY(command string, args []string, cols, rows uint16) (*ptyHandle, error) {
	c, r := normalizedSize(cols, rows)

	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true,
		Setsid:  true,
	}

	ws := &pty.Winsize{
		Rows: r,
		Cols: c,
	}

	pt, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	handle := &ptyHandle{
		stdin:  pt,
		stdout: pt,
		resize: func(cols, rows uint16) error {
			if cols == 0 && rows == 0 {
				return nil
			}
			c, r := normalizedSize(cols, rows)
			return pty.Setsize(pt, &pty.Winsize{Rows: r, Cols: c})
		},
		close: func() error {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
			return pt.Close()
		},
		wait: func() (int, error) {
			err := cmd.Wait()
			exitCode := 0
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			return exitCode, err
		},
	}

	return handle, nil
}
