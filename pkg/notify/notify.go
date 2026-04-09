package notify

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Config holds email notification settings derived from rancid.conf.
type Config struct {
	SendMail    string   // path to sendmail; defaults to /usr/sbin/sendmail
	Recipients  []string // primary recipients (e.g. "rancid-core")
	Subject     string
	MailDomain  string // appended to recipients (e.g. "@example.com")
	MailHeaders string // additional headers, literal \n separated
	MailOpts    string // extra flags for sendmail
}

// BuildMessage constructs the full email message (headers + body) as a string.
// Exported for testing without invoking sendmail.
func BuildMessage(cfg Config, diff []byte) string {
	var sb strings.Builder

	// Headers: use custom if provided, otherwise defaults
	headers := "Precedence: bulk\nAuto-submitted: auto-generated\nX-Auto-Response-Suppress: All"
	if cfg.MailHeaders != "" {
		headers = cfg.MailHeaders
	}
	for _, h := range strings.Split(headers, "\n") {
		sb.WriteString(h + "\n")
	}

	sb.WriteString(fmt.Sprintf("Date: %s\n", time.Now().Format(time.RFC1123Z)))
	sb.WriteString(fmt.Sprintf("Subject: %s\n", cfg.Subject))

	for _, r := range cfg.Recipients {
		sb.WriteString(fmt.Sprintf("To: %s%s\n", r, cfg.MailDomain))
	}
	sb.WriteString("\n")
	sb.Write(diff)
	return sb.String()
}

// SendDiff sends diff via sendmail. Skips silently if diff is empty.
func SendDiff(cfg Config, diff []byte) error {
	if len(diff) == 0 {
		return nil
	}
	bin := cfg.SendMail
	if bin == "" {
		bin = "/usr/sbin/sendmail"
	}

	args := []string{"-t"}
	if cfg.MailOpts != "" {
		args = append(args, strings.Fields(cfg.MailOpts)...)
	}

	cmd := exec.Command(bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("sendmail pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sendmail start: %w", err)
	}
	_, _ = io.WriteString(stdin, BuildMessage(cfg, diff))
	stdin.Close()
	return cmd.Wait()
}