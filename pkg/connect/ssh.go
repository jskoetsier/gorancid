package connect

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"

	"gorancid/pkg/config"
)

// defaultPrompt matches most Cisco/Juniper/Fortinet prompts:
//
//	Router#   Router>   user@host>   FW-01 #
var defaultPrompt = regexp.MustCompile(`[\r\n][\w./-]+[>#]\s*$`)

// SSHSession implements Session using golang.org/x/crypto/ssh.
// It connects, allocates a PTY, detects the prompt, and runs commands interactively.
type SSHSession struct {
	Host  string
	Port  int
	Creds config.Credentials
	Opts  DeviceOpts

	client    *ssh.Client
	session   *ssh.Session
	stdin     io.WriteCloser
	stdout    io.Reader
	prompt    *regexp.Regexp // detected prompt pattern
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
		User:            s.Creds.Username,
		Auth:            sshAuthMethods(s.Creds),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // RANCID doesn't check host keys
		Timeout:         15 * time.Second,
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

	rows, cols := terminalSize()
	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	// Request PTY (required for network devices)
	if err := s.session.RequestPty(termType, rows, cols, ssh.TerminalModes{
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

	// Interactive shells need a live reader. Avoid assigning Stdout/Stderr directly
	// because ssh.Session forbids mixing those with StdoutPipe/StderrPipe.
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

	// Mark the session as connected before issuing enable/setup commands.
	s.connected = true

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
	return nil
}

func sshAuthMethods(creds config.Credentials) []ssh.AuthMethod {
	if creds.Password == "" {
		return nil
	}
	return []ssh.AuthMethod{
		ssh.Password(creds.Password),
		ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = creds.Password
			}
			return answers, nil
		}),
	}
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

// Interact attaches an already-connected interactive shell to the provided streams.
func (s *SSHSession) Interact(ctx context.Context, in io.Reader, out io.Writer) error {
	if !s.connected {
		return fmt.Errorf("not connected")
	}

	var (
		restore    func() error
		resizeStop func()
	)
	if inFile, ok := in.(*os.File); ok && isTerminal(int(inFile.Fd())) {
		state, err := makeRaw(int(inFile.Fd()))
		if err != nil {
			return fmt.Errorf("terminal raw mode: %w", err)
		}
		restore = func() error { return restoreTerminal(int(inFile.Fd()), state) }
		defer restore()

		if outFile, ok := out.(*os.File); ok {
			resizeStop = s.watchWindowChanges(ctx, inFile, outFile)
			defer resizeStop()
		}
	}

	waitCh := make(chan error, 1)
	go func() {
		_, _ = io.Copy(out, s.stdout)
		waitCh <- s.session.Wait()
	}()

	go func() {
		_, _ = io.Copy(s.stdin, in)
		_ = s.stdin.Close()
	}()

	select {
	case err := <-waitCh:
		if err == io.EOF {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func terminalSize() (rows, cols int) {
	rows, cols = 24, 80
	if isTerminal(int(os.Stdout.Fd())) {
		if c, r, err := getSize(int(os.Stdout.Fd())); err == nil && c > 0 && r > 0 {
			return r, c
		}
	}
	if isTerminal(int(os.Stdin.Fd())) {
		if c, r, err := getSize(int(os.Stdin.Fd())); err == nil && c > 0 && r > 0 {
			return r, c
		}
	}
	return rows, cols
}

func (s *SSHSession) watchWindowChanges(ctx context.Context, inFile, outFile *os.File) func() {
	resizeCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(resizeCh, syscall.SIGWINCH)

	apply := func() {
		fd := int(outFile.Fd())
		if !isTerminal(fd) {
			fd = int(inFile.Fd())
		}
		cols, rows, err := getSize(fd)
		if err == nil && cols > 0 && rows > 0 {
			_ = s.session.WindowChange(rows, cols)
		}
	}
	apply()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-resizeCh:
				apply()
			}
		}
	}()

	return func() {
		signal.Stop(resizeCh)
		close(done)
	}
}

func isTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, ioctlReadTermios())
	return err == nil
}

func makeRaw(fd int) (*unix.Termios, error) {
	oldState, err := unix.IoctlGetTermios(fd, ioctlReadTermios())
	if err != nil {
		return nil, err
	}
	newState := *oldState
	newState.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	newState.Oflag &^= unix.OPOST
	newState.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	newState.Cflag &^= unix.CSIZE | unix.PARENB
	newState.Cflag |= unix.CS8
	newState.Cc[unix.VMIN] = 1
	newState.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios(), &newState); err != nil {
		return nil, err
	}
	return oldState, nil
}

func restoreTerminal(fd int, state *unix.Termios) error {
	return unix.IoctlSetTermios(fd, ioctlWriteTermios(), state)
}

func getSize(fd int) (cols, rows int, err error) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}
	return int(ws.Col), int(ws.Row), nil
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
