package connect

import (
	"context"
	"errors"
	"testing"

	"gorancid/pkg/config"
)

func TestSSHSessionNotConnected(t *testing.T) {
	s := &SSHSession{}
	_, err := s.RunCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestSSHSessionClose(t *testing.T) {
	s := &SSHSession{}
	err := s.Close()
	if err != nil {
		t.Errorf("Close on unconnected session should not error: %v", err)
	}
}

func TestNewSessionRequiresNative(t *testing.T) {
	_, err := NewSession("sw-01", 22, config.Credentials{}, DeviceOpts{}, "clogin", false)
	if !errors.Is(err, ErrNoNativeTransport) {
		t.Fatalf("expected ErrNoNativeTransport when preferNative=false, got %v", err)
	}
}

func TestNewSessionSSH(t *testing.T) {
	s, err := NewSession("sw-01", 22, config.Credentials{}, DeviceOpts{}, "clogin", true)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, ok := s.(*SSHSession); !ok {
		t.Fatalf("expected *SSHSession, got %T", s)
	}
}

func TestNewSessionTelnet(t *testing.T) {
	s, err := NewSession("sw-01", 22, config.Credentials{Methods: []string{"telnet"}}, DeviceOpts{}, "clogin", true)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, ok := s.(*TelnetSession); !ok {
		t.Fatalf("expected *TelnetSession, got %T", s)
	}
}

func TestNewSessionPrefersFirstMethod(t *testing.T) {
	s, err := NewSession("sw-01", 22, config.Credentials{Methods: []string{"telnet", "ssh"}}, DeviceOpts{}, "clogin", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*TelnetSession); !ok {
		t.Fatalf("expected telnet first, got %T", s)
	}
}

func TestNewSessionErrorWhenNoTransportMethod(t *testing.T) {
	_, err := NewSession("sw-01", 22, config.Credentials{Methods: []string{"rsh"}}, DeviceOpts{}, "clogin", true)
	if !errors.Is(err, ErrNoNativeTransport) {
		t.Fatalf("expected ErrNoNativeTransport, got %v", err)
	}
}

func TestSSHAuthMethods(t *testing.T) {
	if methods := sshAuthMethods(config.Credentials{}); len(methods) != 0 {
		t.Fatalf("expected no auth methods without password, got %d", len(methods))
	}
	if methods := sshAuthMethods(config.Credentials{Password: "secret"}); len(methods) != 2 {
		t.Fatalf("expected password and keyboard-interactive methods, got %d", len(methods))
	}
}
