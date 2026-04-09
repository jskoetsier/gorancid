package connect_test

import (
	"context"
	"testing"

	"gorancid/pkg/config"
	"gorancid/pkg/connect"
)

func TestSSHSessionNotConnected(t *testing.T) {
	s := &connect.SSHSession{}
	_, err := s.RunCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestSSHSessionClose(t *testing.T) {
	s := &connect.SSHSession{}
	err := s.Close()
	if err != nil {
		t.Errorf("Close on unconnected session should not error: %v", err)
	}
}

func TestExpectSessionConnect(t *testing.T) {
	e := &connect.ExpectSession{}
	err := e.Connect(context.Background())
	if err != nil {
		t.Errorf("ExpectSession.Connect should be no-op: %v", err)
	}
}

func TestExpectSessionClose(t *testing.T) {
	e := &connect.ExpectSession{}
	err := e.Close()
	if err != nil {
		t.Errorf("ExpectSession.Close should be no-op: %v", err)
	}
}

func TestNewSessionExpect(t *testing.T) {
	s := connect.NewSession("sw-01", 22, config.Credentials{}, connect.DeviceOpts{}, "clogin", false)
	if _, ok := s.(*connect.ExpectSession); !ok {
		t.Error("expected ExpectSession when goParserAvailable=false")
	}
}

func TestNewSessionSSH(t *testing.T) {
	s := connect.NewSession("sw-01", 22, config.Credentials{}, connect.DeviceOpts{}, "clogin", true)
	if _, ok := s.(*connect.SSHSession); !ok {
		t.Error("expected SSHSession when goParserAvailable=true")
	}
}