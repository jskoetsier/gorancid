package connect

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

	client     *ssh.Client
	session    *ssh.Session
	stdin      io.WriteCloser
	stdout     io.Reader
	prompt     *regexp.Regexp // detected prompt pattern
	connected  bool

	// Background reader goroutine fields — one goroutine reads s.stdout
	// into readBuf for the session lifetime. readUntilPrompt and
	// Interact both consume from this buffer, avoiding the race where
	// a leaked pump goroutine from a previous call steals data.
	readMu   sync.Mutex
	readBuf  []byte
	readPos  int // bytes consumed by all readers
	readCh   chan struct{}
	readErr  error
	readDone bool
}

// Connect dials the SSH server, authenticates, starts an interactive shell with a PTY,
// and waits for the initial prompt.
func (s *SSHSession) Connect(ctx context.Context) error {
	if s.Port == 0 {
		s.Port = 22
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	// Build SSH config — include legacy key exchange algorithms and CBC
	// ciphers for older network devices (e.g., Cisco IOS on aging hardware)
	// that don't support modern crypto. This is a parity requirement with
	// stock RANCID which uses an unrestricted OpenSSH client.
	sshConfig := &ssh.ClientConfig{
		User:            s.Creds.Username,
		Auth:            sshAuthMethods(s.Creds),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // RANCID doesn't check host keys
		Timeout:         15 * time.Second,
		Config: ssh.Config{
			KeyExchanges: []string{
				"mlkem768x25519-sha256",
				"curve25519-sha256",
				"curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256",
				"ecdh-sha2-nistp384",
				"ecdh-sha2-nistp521",
				"diffie-hellman-group14-sha256",
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group-exchange-sha1",
				"diffie-hellman-group1-sha1",
			},
			Ciphers: []string{
				"aes128-gcm@openssh.com",
				"aes256-gcm@openssh.com",
				"chacha20-poly1305@openssh.com",
				"aes128-ctr",
				"aes192-ctr",
				"aes256-ctr",
				"aes128-cbc",
				"aes192-cbc",
				"aes256-cbc",
				"3des-cbc",
			},
		},
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

	// Start a background reader that continuously buffers stdout.
	// This avoids the per-readUntilPrompt goroutine leak that caused
	// a race where an old goroutine consumed data meant for the next call.
	s.readCh = make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.stdout.Read(buf)
			s.readMu.Lock()
			if n > 0 {
				s.readBuf = append(s.readBuf, buf[:n]...)
			}
			if err != nil {
				s.readErr = err
				s.readDone = true
				s.readMu.Unlock()
				select {
				case s.readCh <- struct{}{}:
				default:
				}
				return
			}
			s.readMu.Unlock()
			select {
			case s.readCh <- struct{}{}:
			default:
			}
		}
	}()

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
			log.Printf("setup command %q on %s: %v", cmd, s.Host, err)
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
		// Copy buffered stdout to the user's terminal in real-time.
		for {
			s.readMu.Lock()
			for s.readPos >= len(s.readBuf) && !s.readDone {
				s.readMu.Unlock()
				select {
				case <-s.readCh:
				case <-ctx.Done():
					return
				}
				s.readMu.Lock()
			}
			if s.readPos < len(s.readBuf) {
				data := s.readBuf[s.readPos:]
				s.readPos = len(s.readBuf)
				s.readMu.Unlock()
				out.Write(data)
				continue
			}
			done := s.readDone
			s.readMu.Unlock()
			if done {
				break
			}
		}
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

var rePagerPrompt = regexp.MustCompile(`--More--| --More-- `)

// readUntilPrompt reads from the session stdout until the prompt pattern matches.
// When a pager prompt (like --More--) is detected, it sends a space to continue output.
//
// The underlying ssh.Session stdout is a backpressure-bound pipe that does NOT
// honour SetReadDeadline. A single background goroutine (started in Connect)
// continuously reads from the pipe into a shared buffer. This function consumes
// from that buffer, avoiding the race where a leaked per-call pump goroutine
// stole data meant for the next invocation.
func (s *SSHSession) readUntilPrompt(ctx context.Context, buf []byte, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	var accumulated []byte

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return accumulated, ErrTimeout
		}

		select {
		case <-ctx.Done():
			return accumulated, ctx.Err()
		default:
		}

		// Consume any new data produced by the background reader.
		s.readMu.Lock()
		if len(s.readBuf) > s.readPos {
			accumulated = append(accumulated, s.readBuf[s.readPos:]...)
			s.readPos = len(s.readBuf)
		}
		readErr := s.readErr
		readDone := s.readDone
		s.readMu.Unlock()

		// Check for pager prompts and send space to continue.
		if len(accumulated) > 0 {
			cleaned := s.cleanANSI(accumulated)
			if rePagerPrompt.Match(cleaned) {
				fmt.Fprint(s.stdin, " ")
				accumulated = rePagerPrompt.ReplaceAll(accumulated, nil)
				continue
			}
			if s.prompt.Match(cleaned) {
				return accumulated, nil
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				return accumulated, nil
			}
			return accumulated, readErr
		}
		if readDone {
			return accumulated, nil
		}

		// Wait for new data, context cancellation, or timeout.
		select {
		case <-ctx.Done():
			return accumulated, ctx.Err()
		case <-time.After(remaining):
			return accumulated, ErrTimeout
		case <-s.readCh:
			// loop and consume fresh data
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

// cleanANSIBytes strips ANSI escape sequences from device output.
func cleanANSIBytes(data []byte) []byte {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return re.ReplaceAll(data, nil)
}

// cleanANSI strips ANSI escape sequences from the output.
func (s *SSHSession) cleanANSI(data []byte) []byte {
	return cleanANSIBytes(data)
}

// SCPDownload downloads a file from the remote device using the SCP protocol.
// It opens a new SSH session on the existing client connection and runs
// "scp -f <remotePath>" to transfer the file. This is used for devices like
// FortiGate where downloading the config file via SCP is more reliable than
// running "show full-configuration" interactively.
func (s *SSHSession) SCPDownload(ctx context.Context, remotePath string) ([]byte, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	scpSession, err := s.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("scp session: %w", err)
	}
	defer scpSession.Close()

	stdin, err := scpSession.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("scp stdin: %w", err)
	}
	stdout, err := scpSession.StdoutPipe()
 if err != nil {
		return nil, fmt.Errorf("scp stdout: %w", err)
	}

	if err := scpSession.Start(fmt.Sprintf("scp -f %s", remotePath)); err != nil {
		return nil, fmt.Errorf("scp start: %w", err)
	}

	timeout := s.Opts.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	deadline := time.Now().Add(timeout)

	// Send initial acknowledgment
	if _, err := stdin.Write([]byte{0}); err != nil {
		return nil, fmt.Errorf("scp ack: %w", err)
	}

	// Read SCP file header: C<mode> <size> <filename>\n
	var header []byte
	buf := make([]byte, 1)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return nil, ErrTimeout
		}
		n, err := stdout.Read(buf)
		if n > 0 {
			header = append(header, buf[0])
			if buf[0] == '\n' {
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("scp header read: %w", err)
		}
	}

	// Parse header: C<mode> <size> <filename>
	headerStr := strings.TrimSpace(string(header))
	if len(headerStr) == 0 || headerStr[0] != 'C' {
		return nil, fmt.Errorf("scp: unexpected header %q", headerStr)
	}
	parts := strings.Fields(headerStr[1:]) // skip 'C'
	if len(parts) < 2 {
		return nil, fmt.Errorf("scp: malformed header %q", headerStr)
	}
	fileSize, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("scp: invalid size in header %q: %w", headerStr, err)
	}

	// Acknowledge the header
	if _, err := stdin.Write([]byte{0}); err != nil {
		return nil, fmt.Errorf("scp ack header: %w", err)
	}

	// Read file content
	content := make([]byte, fileSize)
	totalRead := 0
	for totalRead < int(fileSize) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return nil, ErrTimeout
		}
		n, err := stdout.Read(content[totalRead:])
		totalRead += n
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("scp read content: %w", err)
		}
	}

	// Acknowledge end of transfer
	if _, err := stdin.Write([]byte{0}); err != nil {
		return nil, fmt.Errorf("scp ack end: %w", err)
	}

	// Wait for the SCP session to finish
	done := make(chan error, 1)
	go func() {
		done <- scpSession.Wait()
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-done:
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("scp wait: %w", err)
		}
	}

	return content[:totalRead], nil
}
