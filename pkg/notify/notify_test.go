package notify_test

import (
	"strings"
	"testing"

	"gorancid/pkg/notify"
)

func TestBuildMessage(t *testing.T) {
	cfg := notify.Config{
		Recipients: []string{"rancid-core"},
		Subject:    "rancid diffs for core",
		MailDomain: "@example.com",
	}
	msg := notify.BuildMessage(cfg, []byte("--- old\n+++ new\n"))

	if !strings.Contains(msg, "To: rancid-core@example.com") {
		t.Errorf("missing To header, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Subject: rancid diffs for core") {
		t.Errorf("missing Subject header")
	}
	if !strings.Contains(msg, "Precedence: bulk") {
		t.Errorf("missing Precedence header")
	}
	if !strings.Contains(msg, "--- old") {
		t.Errorf("missing diff body")
	}
}

func TestBuildMessageCustomHeaders(t *testing.T) {
	cfg := notify.Config{
		Recipients:  []string{"rancid-core"},
		Subject:     "test",
		MailHeaders: "X-Custom: yes\nPrecedence: list",
	}
	msg := notify.BuildMessage(cfg, []byte("diff"))
	if !strings.Contains(msg, "X-Custom: yes") {
		t.Errorf("missing custom header: %s", msg)
	}
}