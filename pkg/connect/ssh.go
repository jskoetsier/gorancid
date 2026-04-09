package connect

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"gorancid/pkg/config"
)

// defaultPrompt matches most Cisco/Juniper/Fortinet prompts:
//   Router#   Router>   user@host>   FW-01 #
var defaultPrompt = regexp.MustCompile(`[\r\n][\w./-]+[>#]\s*$`)

// SSHSession implements Session using golang.org/x/crypto/ssh.
// It connects, allocates a PTY, detects the prompt, and runs commands interactively.
type SSHSession struct {
	Host  string
	Port  int
	Creds config.Credentials
	Opts  DeviceOpts

	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	prompt  *regexp.Regexp // detected prompt pattern
	connected bool
}

// Connect dials the SSH server, authenticates, starts an interactive shell with a PTY,
// and waits for the initial prompt.
func (s *SSHSession) Connect(ctx context.Context) error {
	if s.Port == 0 {
		s.Port = 22
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	// Build SSH config
	sshConfig := &ssh.ClientConfig{
		User: s.Creds.Username,
		Auth: []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // RANCID doesn't check host keys
		Timeout: 15 * time.Second,
	}
	if s.Creds.Password != "" {
		sshConfig.Auth = append(sshConfig.Auth, ssh.Password(s.Creds.Password))
	}

	// Dial
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoRoute, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}
	s.client = ssh.NewClient(sshConn, chans, reqs)

	// Open session
	s.session, err = s.client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh new session: %w", err)
	}

	// Request PTY (required for network devices)
	if err := s.session.RequestPty("xterm", 0, 200, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}); err != nil {
		return fmt.Errorf("pty request: %w", err)
	}

	// Get stdin pipe
	s.stdin, err = s.session.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	// Combine stdout and stderr
	var stdout, stderr bytes.Buffer
	s.session.Stdout = &stdout
	s.session.Stderr = &stderr
	multiReader := io.MultiReader(&stdout, &stderr)
	_ = multiReader // we'll use a different approach

	// Actually, for interactive shells we need to read from the session's output
	// in real-time. Let's use a pipe approach.
	s.stdout, err = s.session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	// Start shell
	if err := s.session.Shell(); err != nil {
		return fmt.Errorf("shell start: %w", err)
	}

	// Set up prompt pattern
	s.prompt = defaultPrompt
	if s.Opts.PromptPattern != "" {
		s.prompt = regexp.MustCompile(s.Opts.PromptPattern)
	}

	// Read until we see the initial prompt
	buf := make([]byte, 4096)
	timeout := s.Opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	_, err = s.readUntilPrompt(ctx, buf, timeout)
	if err != nil {
		return fmt.Errorf("waiting for initial prompt: %w", err)
	}

	// Enter enable mode if configured
	if s.Opts.EnableCmd != "" {
		if _, err := s.RunCommand(ctx, s.Opts.EnableCmd); err != nil {
			return fmt.Errorf("enable: %w", err)
		}
		// Some devices prompt for the enable password
		if s.Creds.EnablePwd != "" {
			// Send the enable password
			fmt.Fprintln(s.stdin, s.Creds.EnablePwd)
			_, err = s.readUntilPrompt(ctx, buf, timeout)
			if err != nil {
				return fmt.Errorf("enable password: %w", err)
			}
		}
	}

	// Run setup commands (terminal length 0, etc.)
	for _, cmd := range s.Opts.SetupCommands {
		if _, err := s.RunCommand(ctx, cmd); err != nil {
			// Setup commands are best-effort — some devices don't support them
			continue
		}
	}

	s.connected = true
	return nil
}

// RunCommand sends cmd, reads until the prompt, and returns the output
// with command echo and prompt stripped.
func (s *SSHSession) RunCommand(ctx context.Context, cmd string) ([]byte, error) {
	if !s.connected {
		return nil, fmt.Errorf("not connected")
	}

	timeout := s.Opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Send the command
	fmt.Fprintln(s.stdin, cmd)

	buf := make([]byte, 4096)
	output, err := s.readUntilPrompt(ctx, buf, timeout)
	if err != nil {
		return output, err
	}

	// Strip the command echo (first line) and the prompt (last line)
	return s.stripEchoAndPrompt(output, cmd), nil
}

// Close shuts down the SSH session and client.
func (s *SSHSession) Close() error {
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.session != nil {
		s.session.Close()
	}
	if s.client != nil {
		s.client.Close()
	}
	s.connected = false
	return nil
}

// readUntilPrompt reads from the session stdout until the prompt pattern matches.
func (s *SSHSession) readUntilPrompt(ctx context.Context, buf []byte, timeout time.Duration) ([]byte, error) {
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

		n, err := s.stdout.Read(buf)
		if n > 0 {
			accumulated = append(accumulated, buf[:n]...)

			// Check for prompt
			cleaned := s.cleanANSI(accumulated)
			if s.prompt.Match(cleaned) {
				return accumulated, nil
			}
		}
		if err != nil && err != io.EOF {
			// Check for timeout — just continue reading
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if err == io.EOF {
				return accumulated, nil
			}
			return accumulated, err
		}
	}
}

// stripEchoAndPrompt removes the command echo line and the final prompt from output.
func (s *SSHSession) stripEchoAndPrompt(output []byte, cmd string) []byte {
	lines := strings.Split(string(output), "\n")

	// Remove the first line if it matches the command echo
	start := 0
	if len(lines) > 0 && strings.Contains(strings.TrimSpace(lines[0]), cmd) {
		start = 1
	}

	// Remove the last line if it looks like a prompt
	end := len(lines)
	if end > start {
		last := strings.TrimSpace(lines[end-1])
		if s.prompt.MatchString(last) || (len(last) > 0 && (strings.HasSuffix(last, "#") || strings.HasSuffix(last, ">"))) {
			end--
		}
		// Also remove second-to-last if it's just a prompt fragment
		if end > start {
			secondLast := strings.TrimSpace(lines[end-1])
			if secondLast == "" {
				end--
			}
		}
	}

	return []byte(strings.Join(lines[start:end], "\n"))
}

// cleanANSI strips ANSI escape sequences from the output.
func (s *SSHSession) cleanANSI(data []byte) []byte {
	// Simple ANSI escape sequence filter
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return re.ReplaceAll(data, nil)
}