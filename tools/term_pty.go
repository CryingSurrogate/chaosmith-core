package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultPTYCols uint16 = 80
	defaultPTYRows uint16 = 24

	outputSettleDelay = 50 * time.Millisecond
)

type PTYInput struct {
	Action        string   `json:"action,omitempty" jsonschema:"open, write, read, resize, or close. Call read after sending commands or opening a new PTY."`
	SessionID     string   `json:"sessionId,omitempty" jsonschema:"identifier of an existing PTY session"`
	Command       string   `json:"command,omitempty" jsonschema:"optional command to execute when opening a new PTY; prefer default (the host shell)"`
	Args          []string `json:"args,omitempty" jsonschema:"arguments passed to the PTY command on open"`
	Data          string   `json:"data,omitempty" jsonschema:"payload written to the PTY when action=write"`
	AppendNewline *bool    `json:"appendNewline,omitempty" jsonschema:"when writing, append a newline (defaults to true if data present)"`
	Rows          uint16   `json:"rows,omitempty" jsonschema:"terminal rows for open/resize"`
	Cols          uint16   `json:"cols,omitempty" jsonschema:"terminal columns for open/resize"`
	Force         bool     `json:"force,omitempty" jsonschema:"when opening, terminate any existing PTY first"`
}

type PTYOutput struct {
	SessionID string `json:"sessionId" jsonschema:"MCP session id controlling this PTY"`
	Output    string `json:"output,omitempty" jsonschema:"new data captured from the PTY since the last call"`
	Plain     string `json:"plain,omitempty" jsonschema:"output with ANSI escape sequences stripped"`
	Started   bool   `json:"started,omitempty" jsonschema:"true if a PTY was started by this call"`
	Closed    bool   `json:"closed,omitempty" jsonschema:"true if the PTY was closed by this call"`
	Exited    bool   `json:"exited,omitempty" jsonschema:"true if the PTY process has exited"`
	ExitCode  int    `json:"exitCode,omitempty" jsonschema:"exit code reported by the PTY process"`
	Error     string `json:"error,omitempty" jsonschema:"error message when the action failed"`
}

type ptyHandle struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	resize func(uint16, uint16) error
	close  func() error
	wait   func() (int, error)
}

type ptySession struct {
	id      string
	handle  *ptyHandle
	onExit  func()
	done    chan struct{}
	closeMu sync.Once

	writeMu sync.Mutex

	outputMu sync.Mutex
	output   bytes.Buffer

	exitMu   sync.Mutex
	exitCode int
	exitErr  error
	exited   bool
	closed   bool

	readErrMu sync.Mutex
	readErr   error

	updateCh chan struct{}
}

func newPTYSession(id string, handle *ptyHandle, onExit func()) *ptySession {
	s := &ptySession{
		id:       id,
		handle:   handle,
		onExit:   onExit,
		done:     make(chan struct{}),
		updateCh: make(chan struct{}, 1),
	}
	go s.readLoop()
	go s.waitLoop()
	return s
}

func (s *ptySession) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.handle.stdout.Read(buf)
		if n > 0 {
			s.outputMu.Lock()
			s.output.Write(buf[:n])
			s.outputMu.Unlock()
			s.notifyUpdate()
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !isClosedPipe(err) {
				s.recordReadError(err)
			}
			return
		}
	}
}

func (s *ptySession) waitLoop() {
	exitCode, err := s.handle.wait()

	s.exitMu.Lock()
	s.exitCode = exitCode
	if err != nil {
		s.exitErr = err
	}
	s.exited = true
	s.closed = true
	s.exitMu.Unlock()

	close(s.done)
	s.notifyUpdate()
	if s.onExit != nil {
		s.onExit()
	}
}

func (s *ptySession) write(data string, appendNewline bool) error {
	if data == "" && !appendNewline {
		return nil
	}
	if appendNewline {
		lineEnding := "\n"
		if runtime.GOOS == "windows" {
			lineEnding = "\r\n"
		}
		if !strings.HasSuffix(data, lineEnding) {
			data += lineEnding
		}
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.isTerminalClosed() {
		return fmt.Errorf("pty session is closed")
	}

	_, err := io.WriteString(s.handle.stdin, data)
	return err
}

func (s *ptySession) resize(cols, rows uint16) error {
	if s.handle.resize == nil {
		return fmt.Errorf("resize not supported on this platform")
	}
	return s.handle.resize(cols, rows)
}

func (s *ptySession) close() error {
	var err error
	s.closeMu.Do(func() {
		s.exitMu.Lock()
		s.closed = true
		s.exitMu.Unlock()
		err = s.handle.close()
	})
	return err
}

func (s *ptySession) waitForExit(timeout time.Duration) bool {
	if timeout <= 0 {
		<-s.done
		return true
	}
	select {
	case <-s.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (s *ptySession) drainOutput() string {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	if s.output.Len() == 0 {
		return ""
	}
	data := s.output.String()
	s.output.Reset()
	return data
}

func (s *ptySession) status() (exited bool, exitCode int, exitErr error) {
	s.exitMu.Lock()
	defer s.exitMu.Unlock()
	return s.exited, s.exitCode, s.exitErr
}

func (s *ptySession) peekReadError() error {
	s.readErrMu.Lock()
	defer s.readErrMu.Unlock()
	return s.readErr
}

func (s *ptySession) recordReadError(err error) {
	s.readErrMu.Lock()
	defer s.readErrMu.Unlock()
	s.readErr = err
}

func (s *ptySession) isTerminalClosed() bool {
	s.exitMu.Lock()
	defer s.exitMu.Unlock()
	return s.closed
}

func (s *ptySession) hasBufferedOutput() bool {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	return s.output.Len() > 0
}

func (s *ptySession) notifyUpdate() {
	if s.updateCh == nil {
		return
	}
	select {
	case s.updateCh <- struct{}{}:
	default:
	}
}

func (s *ptySession) waitForQuiet(timeout time.Duration) {
	if timeout <= 0 || s.updateCh == nil {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-s.updateCh:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(timeout)
		case <-timer.C:
			return
		}
	}
}

var ptyRegistry = struct {
	sync.Mutex
	sessions map[string]*ptySession
}{
	sessions: make(map[string]*ptySession),
}

func storeSession(id string, session *ptySession) {
	ptyRegistry.Lock()
	ptyRegistry.sessions[id] = session
	ptyRegistry.Unlock()
}

func getSession(id string) *ptySession {
	ptyRegistry.Lock()
	defer ptyRegistry.Unlock()
	return ptyRegistry.sessions[id]
}

func removeSession(id string, target *ptySession) {
	ptyRegistry.Lock()
	defer ptyRegistry.Unlock()
	if existing, ok := ptyRegistry.sessions[id]; ok && existing == target {
		delete(ptyRegistry.sessions, id)
	}
}

func ExecPTY(_ context.Context, req *mcp.CallToolRequest, input PTYInput) (*mcp.CallToolResult, PTYOutput, error) {
	sessionID := resolveSessionID(req, input.SessionID)
	if sessionID == "" {
		return nil, PTYOutput{}, fmt.Errorf("session id is required for interactive PTYs")
	}

	session := getSession(sessionID)
	action := normalizeAction(input.Action, session != nil, input)

	if action == "" {
		return nil, PTYOutput{}, fmt.Errorf("action is required")
	}

	output := PTYOutput{SessionID: sessionID}
	var remove bool
	awaitOutput := false

	switch action {
	case "open":
		if session != nil && !input.Force {
			output.Error = "a PTY is already active; use force=true to replace it"
			return nil, output, nil
		}
		if session != nil {
			_ = session.close()
			session.waitForExit(500 * time.Millisecond)
			removeSession(sessionID, session)
			session = nil
		}
		handle, startErr := startPlatformPTY(resolveCommand(input.Command), input.Args, input.Cols, input.Rows)
		if startErr != nil {
			output.Error = startErr.Error()
			return nil, output, nil
		}

		var created *ptySession
		created = newPTYSession(sessionID, handle, func() { removeSession(sessionID, created) })
		storeSession(sessionID, created)
		session = created
		output.Started = true
		awaitOutput = true

	case "write":
		if session == nil {
			output.Error = "no active PTY for this session"
			return nil, output, nil
		}
		appendNL := true
		if input.AppendNewline != nil {
			appendNL = *input.AppendNewline
		} else if input.Data == "" {
			appendNL = false
		}
		if writeErr := session.write(input.Data, appendNL); writeErr != nil {
			output.Error = writeErr.Error()
		} else {
			awaitOutput = true
		}

	case "resize":
		if session == nil {
			output.Error = "no active PTY for this session"
			return nil, output, nil
		}
		if resizeErr := session.resize(input.Cols, input.Rows); resizeErr != nil {
			output.Error = resizeErr.Error()
		} else if input.Rows != 0 || input.Cols != 0 {
			awaitOutput = true
		}

	case "close":
		if session == nil {
			output.Error = "no active PTY for this session"
			return nil, output, nil
		}
		if closeErr := session.close(); closeErr != nil {
			output.Error = closeErr.Error()
		}
		session.waitForExit(500 * time.Millisecond)
		output.Closed = true
		awaitOutput = true
		remove = true

	case "read":
		// no-op: we just fall through to collect buffered output
		awaitOutput = true

	default:
		return nil, PTYOutput{}, fmt.Errorf("unknown action %q", action)
	}

	if session != nil {
		if action == "open" && (input.Rows != 0 || input.Cols != 0) {
			// ensure the PTY honours the provided size after spawn
			if resizeErr := session.resize(input.Cols, input.Rows); resizeErr != nil {
				if output.Error == "" {
					output.Error = resizeErr.Error()
				}
			}
		}

		waitNeeded := awaitOutput
		if action == "read" && session.hasBufferedOutput() {
			waitNeeded = false
		}
		if waitNeeded {
			session.waitForQuiet(outputSettleDelay)
		}

		outputChunk := session.drainOutput()
		if outputChunk != "" {
			output.Output = outputChunk
			output.Plain = stripANSI(outputChunk)
		}

		if readErr := session.peekReadError(); readErr != nil && output.Error == "" {
			output.Error = readErr.Error()
		}

		exited, exitCode, exitErr := session.status()
		if exited {
			output.Exited = true
			output.ExitCode = exitCode
			if exitErr != nil && output.Error == "" {
				output.Error = exitErr.Error()
			}
			remove = true
		}
	}

	if remove && session != nil {
		removeSession(sessionID, session)
	}

	return nil, output, nil
}

func resolveSessionID(req *mcp.CallToolRequest, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if req == nil || req.Session == nil {
		return ""
	}
	return req.Session.ID()
}

func normalizeAction(action string, hasSession bool, input PTYInput) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "" {
		return action
	}
	if !hasSession {
		return "open"
	}
	if input.Data != "" {
		return "write"
	}
	return "read"
}

func resolveCommand(command string) string {
	if strings.TrimSpace(command) != "" {
		return command
	}
	if runtime.GOOS == "windows" {
		if pwsh := strings.TrimSpace(os.Getenv("PWSH")); pwsh != "" {
			return pwsh
		}
		if pwsh := strings.TrimSpace(os.Getenv("CHAOSMITH_PWSH")); pwsh != "" {
			return pwsh
		}
		if found, err := exec.LookPath("pwsh.exe"); err == nil {
			return found
		}
		const storePwsh = `C:\Program Files\WindowsApps\Microsoft.PowerShellPreview_7.6.4.0_x64__8wekyb3d8bbwe\pwsh.exe`
		if _, err := os.Stat(storePwsh); err == nil {
			return storePwsh
		}
		if shell := strings.TrimSpace(os.Getenv("COMSPEC")); strings.Contains(strings.ToLower(shell), "pwsh") {
			return shell
		}
		return "pwsh.exe"
	}
	if shell := os.Getenv("SHELL"); strings.TrimSpace(shell) != "" {
		return shell
	}
	return "/bin/sh"
}

func startPlatformPTY(command string, args []string, cols, rows uint16) (*ptyHandle, error) {
	switch runtime.GOOS {
	case "windows":
		return startWindowsPTY(command, args, cols, rows)
	default:
		return startUnixPTY(command, args, cols, rows)
	}
}

func normalizedSize(cols, rows uint16) (uint16, uint16) {
	if cols == 0 {
		cols = defaultPTYCols
	}
	if rows == 0 {
		rows = defaultPTYRows
	}
	return cols, rows
}

func isClosedPipe(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "file already closed") || strings.Contains(err.Error(), "use of closed file") || strings.Contains(err.Error(), "read |1: file already closed")
}

var (
	ansiCSI  = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiOSC  = regexp.MustCompile(`\x1b\][^\x07]*(\x07|\x1b\\)`)
	ansiSS3  = regexp.MustCompile(`\x1bO.`)
	ansiCSI2 = regexp.MustCompile(`\x9b[0-9;?]*[ -/]*[@-~]`)
)

func stripANSI(input string) string {
	if input == "" {
		return ""
	}
	s := ansiOSC.ReplaceAllString(input, "")
	s = ansiCSI.ReplaceAllString(s, "")
	s = ansiCSI2.ReplaceAllString(s, "")
	s = ansiSS3.ReplaceAllString(s, "")
	return s
}
