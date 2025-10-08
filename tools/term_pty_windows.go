//go:build windows

package tools

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/ActiveState/termtest/conpty"
)

func startWindowsPTY(command string, args []string, cols, rows uint16) (*ptyHandle, error) {
	c, r := normalizedSize(cols, rows)
	ptyDevice, err := conpty.New(int16(c), int16(r))
	if err != nil {
		return nil, fmt.Errorf("create conpty: %w", err)
	}

	pid, _, err := ptyDevice.Spawn(command, args, &syscall.ProcAttr{
		Env: appendEnv(os.Environ(), "TERM=xterm-256color"),
	})
	if err != nil {
		_ = ptyDevice.Close()
		return nil, fmt.Errorf("spawn process: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		_ = ptyDevice.Close()
		return nil, fmt.Errorf("find process: %w", err)
	}

	handle := &ptyHandle{
		stdin:  ptyDevice.InPipe(),
		stdout: ptyDevice.OutPipe(),
		resize: func(cols, rows uint16) error {
			if cols == 0 && rows == 0 {
				return nil
			}
			c, r := normalizedSize(cols, rows)
			return ptyDevice.Resize(c, r)
		},
		close: func() error {
			_ = process.Kill()
			return ptyDevice.Close()
		},
		wait: func() (int, error) {
			state, err := process.Wait()
			if err != nil {
				return 0, err
			}
			return state.ExitCode(), nil
		},
	}

	return handle, nil
}

func appendEnv(env []string, kv string) []string {
	key := strings.SplitN(kv, "=", 2)[0]
	lowered := strings.ToLower(key)
	for i, existing := range env {
		if strings.HasPrefix(strings.ToLower(existing), lowered+"=") {
			env[i] = kv
			return env
		}
	}
	return append(env, kv)
}
