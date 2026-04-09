package connect

import (
	"context"
	"io"
	"time"

	"gorancid/pkg/config"
)

// DeviceOpts contains device-specific connection parameters that vary by vendor.
type DeviceOpts struct {
	// DeviceType identifies the device type (e.g. "ios", "junos").
	DeviceType string
	// PromptPattern is a regex matching the device's shell prompt.
	// Defaults to `[\r\n][\w./-]+[>#]\s*$` if empty.
	PromptPattern string
	// SetupCommands are sent after login before collecting output
	// (e.g. "terminal length 0" for IOS).
	SetupCommands []string
	// EnableCmd is the command to enter privileged mode (e.g. "enable").
	// Leave empty if not needed.
	EnableCmd string
	// EnablePwd specifies the enable password from .cloginrc.
	EnablePwd string
	// DisablePagingCmd is the command to disable output paging.
	// Many devices need this; it's also included in SetupCommands.
	DisablePagingCmd string
	// Timeout is the per-command timeout. Defaults to 30s.
	Timeout time.Duration
}

// Session is the interface for interacting with a network device.
type Session interface {
	// Connect establishes the session (SSH, Expect subprocess, etc.).
	Connect(ctx context.Context) error
	// RunCommand sends cmd and returns the output (without command echo or prompt).
	RunCommand(ctx context.Context, cmd string) ([]byte, error)
	// Close terminates the session.
	Close() error
}

// NewSession returns the appropriate Session implementation:
//   - If a Go parser is registered for deviceType, returns an SSHSession.
//   - Otherwise, returns an ExpectSession (shells out to clogin/jlogin/etc.).
func NewSession(host string, port int, creds config.Credentials, opts DeviceOpts, loginScript string, goParserAvailable bool) Session {
	if goParserAvailable {
		return &SSHSession{
			Host:  host,
			Port:  port,
			Creds: creds,
			Opts:  opts,
		}
	}
	return &ExpectSession{
		Host:        host,
		LoginScript: loginScript,
		DeviceType:  opts.DeviceType,
	}
}

// readUntilPrompt reads from r until the prompt pattern is matched or ctx expires.
// Returns the full output read (including prompt) and any error.
func readUntil(ctx context.Context, r io.Reader, buf []byte, match func([]byte) int, timeout time.Duration) ([]byte, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)

	var accumulated []byte
	for {
		select {
		case <-ctx.Done():
			return accumulated, ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return accumulated, ErrTimeout
		}

		n, err := r.Read(buf)
		if n > 0 {
			accumulated = append(accumulated, buf[:n]...)
			if idx := match(accumulated); idx >= 0 {
				return accumulated[:idx], nil
			}
		}
		if err != nil {
			return accumulated, err
		}
	}
}