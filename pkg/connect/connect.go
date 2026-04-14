package connect

import (
	"context"
	"io"
	"strconv"
	"strings"
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

// NewSession returns an SSHSession or TelnetSession based on the first matching
// method in creds.Methods (in order). Empty methods defaults to SSH on port 22.
func NewSession(host string, defaultSSHPort int, creds config.Credentials, opts DeviceOpts, _ string, preferNative bool) (Session, error) {
	if !preferNative {
		return nil, ErrNoNativeTransport
	}
	kind, port, ok := selectNativeTransport(creds.Methods, defaultSSHPort)
	if !ok {
		return nil, ErrNoNativeTransport
	}
	switch kind {
	case "ssh":
		return &SSHSession{
			Host:  host,
			Port:  port,
			Creds: creds,
			Opts:  opts,
		}, nil
	case "telnet":
		return &TelnetSession{
			Host:  host,
			Port:  port,
			Creds: creds,
			Opts:  opts,
		}, nil
	default:
		return nil, ErrNoNativeTransport
	}
}

func selectNativeTransport(methods []string, defaultSSHPort int) (kind string, port int, ok bool) {
	if defaultSSHPort <= 0 {
		defaultSSHPort = 22
	}
	if len(methods) == 0 {
		return "ssh", defaultSSHPort, true
	}
	for _, method := range methods {
		switch {
		case method == "ssh":
			return "ssh", defaultSSHPort, true
		case strings.HasPrefix(method, "ssh:"):
			p, err := strconv.Atoi(strings.TrimPrefix(method, "ssh:"))
			if err == nil && p > 0 {
				return "ssh", p, true
			}
		case method == "telnet":
			return "telnet", 23, true
		case strings.HasPrefix(method, "telnet:"):
			p, err := strconv.Atoi(strings.TrimPrefix(method, "telnet:"))
			if err == nil && p > 0 {
				return "telnet", p, true
			}
		}
	}
	return "", 0, false
}

// readUntil reads from r until the prompt pattern is matched or ctx expires.
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
