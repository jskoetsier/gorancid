package connect

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"gorancid/pkg/config"
)

// ExpectSession implements Session by shelling out to the existing clogin/jlogin
// scripts. Used for device types that don't have a Go parser yet.
type ExpectSession struct {
	Host        string
	LoginScript string // e.g. "clogin", "jlogin"
	DeviceType  string
	Creds       config.Credentials
	Timeout     int // seconds, 0 = default

	output []byte // accumulated output from all commands
}

// Connect is a no-op for Expect sessions — the login script handles everything.
func (e *ExpectSession) Connect(_ context.Context) error {
	return nil
}

// RunCommand runs the login script with -c <command> and returns the output.
// The login script handles SSH/telnet connection, authentication, and command execution.
func (e *ExpectSession) RunCommand(ctx context.Context, cmd string) ([]byte, error) {
	bin := e.LoginScript
	if bin == "" {
		bin = "clogin"
	}

	args := []string{}
	if e.Timeout > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", e.Timeout))
	}
	args = append(args, "-c", cmd, e.Host)

	cmdObj := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmdObj.Stdout = &stdout
	cmdObj.Stderr = &stderr

	err := cmdObj.Run()
	if err != nil {
		return nil, fmt.Errorf("expect %s: %w\n%s", bin, err, stderr.String())
	}

	e.output = append(e.output, stdout.Bytes()...)
	return stdout.Bytes(), nil
}

// RunAll runs the login script with multiple -c commands and returns the full output.
// This is more efficient than calling RunCommand multiple times.
func (e *ExpectSession) RunAll(ctx context.Context, commands []string) ([]byte, error) {
	bin := e.LoginScript
	if bin == "" {
		bin = "clogin"
	}

	args := []string{}
	if e.Timeout > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", e.Timeout))
	}

	// clogin supports multiple -c flags or a single -c with semicolons
	for _, cmd := range commands {
		args = append(args, "-c", cmd)
	}
	args = append(args, e.Host)

	cmdObj := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmdObj.Stdout = &stdout
	cmdObj.Stderr = &stderr

	err := cmdObj.Run()
	if err != nil {
		return nil, fmt.Errorf("expect %s: %w\n%s", bin, err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// Close is a no-op for Expect sessions.
func (e *ExpectSession) Close() error {
	return nil
}

// RunExpectCommand is a convenience function that runs an arbitrary command
// via exec.CommandContext and returns its stdout. Used by cmd/rancid for
// falling back to the Perl rancid script.
func RunExpectCommand(ctx context.Context, name string, args []string, dir string) ([]byte, error) {
	cmdObj := exec.CommandContext(ctx, name, args...)
	cmdObj.Dir = dir
	var stdout, stderr bytes.Buffer
	cmdObj.Stdout = &stdout
	cmdObj.Stderr = &stderr

	err := cmdObj.Run()
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("%s: %w\n%s", name, err, stderr.String())
	}
	return stdout.Bytes(), nil
}