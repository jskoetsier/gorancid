package collect

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorancid/pkg/config"
	"gorancid/pkg/connect"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/parse"
)

type deviceOptsProvider interface {
	DeviceOpts() connect.DeviceOpts
}

type bulkRunner interface {
	RunAll(ctx context.Context, commands []string) ([]byte, error)
}

// GoCollector runs in-process configuration collection using native SSH or Telnet
// transport and registered parsers (same behavior as cmd/rancid).
type GoCollector struct {
	Device     config.Device
	Spec       devicetype.DeviceSpec
	Creds      config.Credentials
	OutDir     string
	FilterOpts parse.FilterOpts
	// Timeout caps SSH command and connect waits; zero defaults to 30s.
	Timeout time.Duration
}

// Run collects from the device and writes the config to OutDir/<hostname>,
// matching the path control-rancid stages for git.
func (c *GoCollector) Run(ctx context.Context) (Result, error) {
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	outPath := filepath.Join(c.OutDir, c.Device.Hostname)
	if err := CollectDevice(ctx, c.Device.Hostname, c.Creds, c.Spec, c.FilterOpts, outPath, c.Timeout); err != nil {
		return Result{
			Hostname: c.Device.Hostname,
			Status:   StatusFailed,
			Error:    err,
		}, nil
	}
	return Result{
		Hostname: c.Device.Hostname,
		Status:   StatusSuccess,
	}, nil
}

// CollectDevice connects, runs the device-type command list, parses output,
// and writes the filtered config to outPath (full file path).
func CollectDevice(ctx context.Context, hostname string, creds config.Credentials, spec devicetype.DeviceSpec, filterOpts parse.FilterOpts, outPath string, timeout time.Duration) error {
	parser, ok := parse.Lookup(spec.Type)
	if !ok {
		return fmt.Errorf("no parser registered for device type %q", spec.Type)
	}

	opts := connect.DeviceOpts{
		DeviceType: spec.Type,
		Timeout:    timeout,
	}
	preferNative := false
	if provider, ok := parser.(deviceOptsProvider); ok {
		opts = provider.DeviceOpts()
		if opts.Timeout == 0 {
			opts.Timeout = timeout
		}
		preferNative = true
	}

	session, err := connect.NewSession(hostname, 22, creds, opts, spec.LoginScript, preferNative)
	if err != nil {
		return err
	}
	if err := session.Connect(ctx); err != nil {
		return fmt.Errorf("connect %s: %w", hostname, err)
	}
	defer session.Close()

	commandList := make([]string, 0, len(spec.Commands))
	for _, cmd := range spec.Commands {
		commandList = append(commandList, cmd.CLI)
	}
	allOutput, err := collectOutput(ctx, session, commandList, hostname)
	if err != nil {
		return fmt.Errorf("collect %s: %w", hostname, err)
	}

	parsed, err := parser.Parse(allOutput, filterOpts)
	if err != nil {
		return fmt.Errorf("parse %s: %w", hostname, err)
	}

	output := strings.Join(parsed.Lines, "\n")
	if err := os.WriteFile(outPath, []byte(output), 0644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	if len(parsed.Metadata) > 0 {
		metaPath := strings.TrimSuffix(outPath, ".new") + ".meta"
		var metaLines []string
		for k, v := range parsed.Metadata {
			metaLines = append(metaLines, fmt.Sprintf("%s: %s", k, v))
		}
		if err := os.WriteFile(metaPath, []byte(strings.Join(metaLines, "\n")+"\n"), 0644); err != nil {
			return fmt.Errorf("write metadata %s: %w", metaPath, err)
		}
	}
	return nil
}

func collectOutput(ctx context.Context, session connect.Session, commands []string, hostname string) ([]byte, error) {
	if bulk, ok := session.(bulkRunner); ok {
		return bulk.RunAll(ctx, commands)
	}

	var allOutput []byte
	for _, cmd := range commands {
		output, err := session.RunCommand(ctx, cmd)
		if err != nil {
			log.Printf("command %q on %s: %v", cmd, hostname, err)
			continue
		}
		allOutput = append(allOutput, output...)
		allOutput = append(allOutput, '\n')
	}
	return allOutput, nil
}
