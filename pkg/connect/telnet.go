package connect

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"gorancid/pkg/config"
)

// Telnet protocol bytes (RFC 854).
const (
	telnetIAC  = 255
	telnetDONT = 254
	telnetDO   = 253
	telnetWONT = 252
	telnetWILL = 251
	telnetSB   = 250
	telnetSE   = 240
)

// TelnetSession implements Session over cleartext Telnet with minimal option
// negotiation suitable for network switches and routers.
type TelnetSession struct {
	Host  string
	Port  int
	Creds config.Credentials
	Opts  DeviceOpts

	conn      net.Conn
	r         *telnetReader
	prompt    *regexp.Regexp
	connected bool
}

// Connect dials the device, completes username/password prompts when present,
// then matches the shell prompt and runs setup commands like SSHSession.
func (s *TelnetSession) Connect(ctx context.Context) error {
	if s.Port == 0 {
		s.Port = 23
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	d := net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoRoute, err)
	}
	s.conn = conn
	s.r = newTelnetReader(conn)

	s.prompt = defaultPrompt
	if s.Opts.PromptPattern != "" {
		s.prompt = regexp.MustCompile(s.Opts.PromptPattern)
	}

	timeout := s.Opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if err := s.drainThroughLogin(ctx, timeout); err != nil {
		_ = s.conn.Close()
		s.conn = nil
		return err
	}

	s.connected = true

	if s.Opts.EnableCmd != "" {
		if _, err := s.RunCommand(ctx, s.Opts.EnableCmd); err != nil {
			return fmt.Errorf("enable: %w", err)
		}
		if s.Creds.EnablePwd != "" {
			fmt.Fprintf(s.conn, "%s\r\n", s.Creds.EnablePwd)
			buf := make([]byte, 4096)
			if _, err := s.readUntilPrompt(ctx, buf, timeout); err != nil {
				return fmt.Errorf("enable password: %w", err)
			}
		}
	}

	for _, cmd := range s.Opts.SetupCommands {
		if _, err := s.RunCommand(ctx, cmd); err != nil {
			continue
		}
	}
	return nil
}

func (s *TelnetSession) drainThroughLogin(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var acc []byte
	tmp := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := s.r.Read(tmp)
		_ = s.conn.SetReadDeadline(time.Time{})
		if n > 0 {
			acc = append(acc, tmp[:n]...)
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() && n == 0 {
				continue
			}
			if err != io.EOF {
				return err
			}
		}
		plain := string(cleanANSIBytes(acc))
		low := strings.ToLower(plain)
		if strings.Contains(low, "username:") || strings.Contains(low, "user name:") {
			u := s.Creds.Username
			if u == "" {
				return fmt.Errorf("%w: device requested username but .cloginrc has none", ErrAuthFailed)
			}
			fmt.Fprintf(s.conn, "%s\r\n", u)
			acc = nil
			continue
		}
		if matchLoginPrompt(low) {
			u := s.Creds.Username
			if u == "" {
				return fmt.Errorf("%w: device requested login but .cloginrc has none", ErrAuthFailed)
			}
			fmt.Fprintf(s.conn, "%s\r\n", u)
			acc = nil
			continue
		}
		if strings.Contains(low, "password:") && !strings.Contains(low, "enable password") {
			if s.Creds.Password == "" {
				return fmt.Errorf("%w: device requested password but .cloginrc has none", ErrAuthFailed)
			}
			fmt.Fprintf(s.conn, "%s\r\n", s.Creds.Password)
			acc = nil
			continue
		}
		if s.prompt.Match(cleanANSIBytes(acc)) {
			return nil
		}
		if err == io.EOF {
			if len(acc) == 0 {
				return ErrNoRoute
			}
			return fmt.Errorf("telnet: connection closed before shell prompt")
		}
	}
}

func matchLoginPrompt(low string) bool {
	i := strings.LastIndex(low, "login:")
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(low[i+len("login:"):])
	return rest == ""
}

// RunCommand sends a line to the device and reads until the next prompt.
func (s *TelnetSession) RunCommand(ctx context.Context, cmd string) ([]byte, error) {
	if !s.connected {
		return nil, fmt.Errorf("not connected")
	}
	timeout := s.Opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if _, err := fmt.Fprintf(s.conn, "%s\r\n", cmd); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	output, err := s.readUntilPrompt(ctx, buf, timeout)
	if err != nil {
		return output, err
	}
	return stripEchoAndPromptTelnet(s.prompt, output, cmd), nil
}

func (s *TelnetSession) readUntilPrompt(ctx context.Context, buf []byte, timeout time.Duration) ([]byte, error) {
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
		_ = s.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := s.r.Read(buf)
		_ = s.conn.SetReadDeadline(time.Time{})
		if n > 0 {
			accumulated = append(accumulated, buf[:n]...)
			if s.prompt.Match(cleanANSIBytes(accumulated)) {
				return accumulated, nil
			}
		}
		if err != nil && err != io.EOF {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return accumulated, err
		}
		if err == io.EOF {
			return accumulated, nil
		}
	}
}

func stripEchoAndPromptTelnet(prompt *regexp.Regexp, output []byte, cmd string) []byte {
	lines := strings.Split(string(output), "\n")
	start := 0
	if len(lines) > 0 && strings.Contains(strings.TrimSpace(lines[0]), cmd) {
		start = 1
	}
	end := len(lines)
	if end > start {
		last := strings.TrimSpace(lines[end-1])
		if prompt.MatchString(last) || (len(last) > 0 && (strings.HasSuffix(last, "#") || strings.HasSuffix(last, ">") || strings.HasSuffix(last, "%"))) {
			end--
		}
		if end > start {
			if strings.TrimSpace(lines[end-1]) == "" {
				end--
			}
		}
	}
	return []byte(strings.Join(lines[start:end], "\n"))
}

// Close closes the TCP connection.
func (s *TelnetSession) Close() error {
	s.connected = false
	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}

// Interact attaches the telnet stream to local stdin/stdout (raw mode when possible).
func (s *TelnetSession) Interact(ctx context.Context, in io.Reader, out io.Writer) error {
	if !s.connected {
		return fmt.Errorf("not connected")
	}
	var restore func() error
	if inFile, ok := in.(*os.File); ok && isTerminal(int(inFile.Fd())) {
		state, err := makeRaw(int(inFile.Fd()))
		if err != nil {
			return fmt.Errorf("terminal raw mode: %w", err)
		}
		restore = func() error { return restoreTerminal(int(inFile.Fd()), state) }
		defer restore()
	}
	waitCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(out, s.r)
		waitCh <- err
	}()
	go func() {
		_, err := io.Copy(s.conn, in)
		_ = s.conn.Close()
		waitCh <- err
	}()
	select {
	case err := <-waitCh:
		if err == nil || err == io.EOF {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- telnet wire reader (IAC handling) ---

type telnetReader struct {
	conn net.Conn
	br   *bufio.Reader
}

func newTelnetReader(conn net.Conn) *telnetReader {
	return &telnetReader{conn: conn, br: bufio.NewReader(conn)}
}

func (t *telnetReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	filled := 0
	for filled < len(p) {
		b, err := t.readPlainByte()
		if err != nil {
			if filled > 0 {
				return filled, nil
			}
			return 0, err
		}
		p[filled] = b
		filled++
	}
	return filled, nil
}

func (t *telnetReader) readPlainByte() (byte, error) {
	for {
		b, err := t.br.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != telnetIAC {
			return b, nil
		}
		cmd, err := t.br.ReadByte()
		if err != nil {
			return 0, err
		}
		if cmd == telnetIAC {
			return telnetIAC, nil
		}
		switch cmd {
		case telnetDO, telnetDONT, telnetWILL, telnetWONT:
			opt, err := t.br.ReadByte()
			if err != nil {
				return 0, err
			}
			t.negotiate(cmd, opt)
		case telnetSB:
			if err := t.skipSubnegotiation(); err != nil {
				return 0, err
			}
		default:
			// ignore unknown IAC command
		}
	}
}

func (t *telnetReader) negotiate(cmd, opt byte) {
	var resp [3]byte
	resp[0] = telnetIAC
	switch cmd {
	case telnetDO:
		// Peer asks us to enable option opt.
		resp[1] = telnetWONT
		resp[2] = opt
		if opt == 3 { // Suppress Go Ahead
			resp[1] = telnetWILL
		}
	case telnetWILL:
		// Peer will enable option opt for itself.
		resp[1] = telnetDONT
		resp[2] = opt
		if opt == 1 { // Echo
			resp[1] = telnetDO
		}
	default:
		return
	}
	_, _ = t.conn.Write(resp[:])
}

func (t *telnetReader) skipSubnegotiation() error {
	const maxSub = 1 << 16
	for n := 0; n < maxSub; n++ {
		b, err := t.br.ReadByte()
		if err != nil {
			return err
		}
		if b == telnetIAC {
			next, err := t.br.ReadByte()
			if err != nil {
				return err
			}
			if next == telnetSE {
				return nil
			}
		}
	}
	return fmt.Errorf("telnet: subnegotiation too long")
}
