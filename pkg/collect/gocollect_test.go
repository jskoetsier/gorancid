package collect

import (
	"context"
	"errors"
	"testing"

	"gorancid/pkg/connect"
	"gorancid/pkg/devicetype"
)

var _ connect.Session = (*mockSession)(nil)

// mockSession implements connect.Session for testing.
type mockSession struct {
	outputs map[string][]byte
	errs    map[string]error
}

func (m *mockSession) Connect(ctx context.Context) error { return nil }
func (m *mockSession) Close() error                    { return nil }
func (m *mockSession) RunCommand(ctx context.Context, cmd string) ([]byte, error) {
	if err, ok := m.errs[cmd]; ok {
		return nil, err
	}
	return m.outputs[cmd], nil
}

// mockBulkSession implements both connect.Session and bulkRunner.
type mockBulkSession struct {
	mockSession
	bulkOut []byte
	bulkErr error
}

func (m *mockBulkSession) RunAll(ctx context.Context, commands []string) ([]byte, error) {
	return m.bulkOut, m.bulkErr
}

func TestIsConfigCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  devicetype.Command
		want bool
	}{
		{"show full-configuration", devicetype.Command{CLI: "show full-configuration", Handler: "ShowConf"}, true},
		{"show configuration", devicetype.Command{CLI: "show configuration", Handler: "ShowConf"}, true},
		{"GETCONF handler", devicetype.Command{CLI: "get system status", Handler: "GetConf"}, true},
		{"SHOWCONF handler", devicetype.Command{CLI: "display config", Handler: "ShowConf"}, true},
		{"show version", devicetype.Command{CLI: "show version", Handler: "ShowVersion"}, false},
		{"get system status", devicetype.Command{CLI: "get system status", Handler: "ShowStatus"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isConfigCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("isConfigCommand(%+v) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestCollectOutputSuccess(t *testing.T) {
	m := &mockSession{
		outputs: map[string][]byte{
			"show version":     []byte("Version 1.0\n"),
			"show running-config": []byte("interface eth0\n"),
		},
	}
	out, err := collectOutput(context.Background(), m, []string{"show version", "show running-config"}, "sw-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "show version\nVersion 1.0\n\nshow running-config\ninterface eth0\n\n"
	if string(out) != want {
		t.Errorf("output = %q, want %q", string(out), want)
	}
}

func TestCollectOutputEmptyCommands(t *testing.T) {
	m := &mockSession{}
	out, err := collectOutput(context.Background(), m, []string{}, "sw-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %q", string(out))
	}
}

func TestCollectOutputCommandError(t *testing.T) {
	m := &mockSession{
		outputs: map[string][]byte{
			"show version": []byte("Version 1.0\n"),
		},
		errs: map[string]error{
			"show running-config": errors.New("timeout"),
		},
	}
	_, err := collectOutput(context.Background(), m, []string{"show version", "show running-config"}, "sw-01")
	if err == nil {
		t.Fatal("expected error for failed command, got nil")
	}
	if !errors.Is(err, errors.New("timeout")) {
		// The error should wrap the underlying cause.
		if !contains(err.Error(), "timeout") {
			t.Errorf("expected error containing 'timeout', got %v", err)
		}
	}
}

func TestCollectOutputBulkRunner(t *testing.T) {
	m := &mockBulkSession{
		bulkOut: []byte("bulk output\n"),
	}
	out, err := collectOutput(context.Background(), m, []string{"cmd1", "cmd2"}, "sw-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "bulk output\n" {
		t.Errorf("output = %q, want %q", string(out), "bulk output\n")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
