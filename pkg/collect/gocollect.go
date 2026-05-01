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

// scpConfigCommands is an optional interface for parsers that support SCP-based
// config download. It returns the CLI command strings that are replaced by SCP.
// Parsers that do not implement this interface have all their commands run via SSH.
type scpConfigCommands interface {
	SCPConfigCommandList() []string
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
func (c *GoCollector) Run(ctx context.Context) Result {
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Spec.Timeout > 0 {
		c.Timeout = c.Spec.Timeout
	}
	outPath := filepath.Join(c.OutDir, c.Device.Hostname)
	if err := CollectDevice(ctx, c.Device.Hostname, c.Creds, c.Spec, c.FilterOpts, outPath, c.Timeout); err != nil {
		return Result{
			Hostname: c.Device.Hostname,
			Status:   StatusFailed,
			Error:    err,
		}
	}
	return Result{
		Hostname: c.Device.Hostname,
		Status:   StatusSuccess,
	}
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
	provider, ok := parser.(deviceOptsProvider)
	if !ok {
		return fmt.Errorf("device type %q has no in-process collection support: parser does not implement DeviceOpts", spec.Type)
	}
	opts = provider.DeviceOpts()
	if opts.Timeout == 0 {
		opts.Timeout = timeout
	}

	session, err := connect.NewSession(hostname, 22, creds, opts)
	if err != nil {
		return err
	}
	if err := session.Connect(ctx); err != nil {
		return fmt.Errorf("connect %s: %w", hostname, err)
	}
	defer session.Close()

	var allOutput []byte

	// If the device supports SCP-based config download, use that for the
	// configuration file and only run SSH commands for metadata collection.
	if opts.SCPConfigFile != "" {
		allOutput, err = collectSCPAndSSH(ctx, session, opts, spec, hostname, parser)
	} else {
		commandList := make([]string, 0, len(spec.Commands))
		for _, cmd := range spec.Commands {
			commandList = append(commandList, cmd.CLI)
		}
		allOutput, err = collectOutput(ctx, session, commandList, hostname)
	}
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
			return allOutput, fmt.Errorf("command %q on %s: %w", cmd, hostname, err)
		}
		// Prepend command echo so parsers can detect section boundaries.
		// RunCommand strips the echo, so we reinject it here.
		allOutput = append(allOutput, []byte(cmd+"\n")...)
		allOutput = append(allOutput, output...)
		allOutput = append(allOutput, '\n')
	}
	return allOutput, nil
}

// collectSCPAndSSH downloads the configuration via SCP and collects metadata
// via interactive SSH commands. This is used for devices like FortiGate where
// "show full-configuration" over SSH is unreliable due to paging and size.
// If SCP download fails (e.g., SCP not enabled on device), it falls back to
// running the config command via SSH.
//
// Which commands are replaced by SCP is determined by the parser: if it
// implements scpConfigCommands, those CLI strings are skipped for SSH and
// deferred to the SCP/SFTP download path. Parsers that don't implement the
// interface have all commands run via SSH (the SCP file is still attempted).
func collectSCPAndSSH(ctx context.Context, session connect.Session, opts connect.DeviceOpts, spec devicetype.DeviceSpec, hostname string, parser parse.Parser) ([]byte, error) {
	var allOutput []byte

	// Build the set of CLI commands that the parser replaces via SCP download.
	configCmdSet := make(map[string]bool)
	if scp, ok := parser.(scpConfigCommands); ok {
		for _, c := range scp.SCPConfigCommandList() {
			configCmdSet[strings.ToLower(c)] = true
		}
	}

	// Collect config commands for potential SSH fallback.
	var configCmds []devicetype.Command

	// Run SSH commands for metadata collection; skip SCP-replaced commands.
	for _, cmd := range spec.Commands {
		if configCmdSet[strings.ToLower(cmd.CLI)] {
			configCmds = append(configCmds, cmd)
			continue
		}
		output, err := session.RunCommand(ctx, cmd.CLI)
		if err != nil {
			log.Printf("command %q on %s: %v", cmd.CLI, hostname, err)
			continue
		}
		allOutput = append(allOutput, []byte(cmd.CLI+"\n")...)
		allOutput = append(allOutput, output...)
		allOutput = append(allOutput, '\n')
	}

	// Try SFTP download first, then SCP, then SSH fallback.
	if sftpDownloader, ok := session.(connect.SFTPDownloader); ok {
		configData, err := sftpDownloader.SFTPDownload(ctx, opts.SCPConfigFile)
		if err == nil {
			for _, cmd := range configCmds {
				allOutput = append(allOutput, []byte(cmd.CLI+"\n")...)
			}
			allOutput = append(allOutput, configData...)
			allOutput = append(allOutput, '\n')
			return allOutput, nil
		}
		log.Printf("sftp download %s on %s: %v — falling back to SCP", opts.SCPConfigFile, hostname, err)
	}
	if scpDownloader, ok := session.(connect.SCPDownloader); ok {
		configData, err := scpDownloader.SCPDownload(ctx, opts.SCPConfigFile)
		if err == nil {
			for _, cmd := range configCmds {
				allOutput = append(allOutput, []byte(cmd.CLI+"\n")...)
			}
			allOutput = append(allOutput, configData...)
			allOutput = append(allOutput, '\n')
			return allOutput, nil
		}
		log.Printf("scp download %s on %s: %v — falling back to SSH", opts.SCPConfigFile, hostname, err)
	}

	// Fallback: run config commands via SSH.
	for _, cmd := range configCmds {
		output, err := session.RunCommand(ctx, cmd.CLI)
		if err != nil {
			return allOutput, fmt.Errorf("command %q on %s: %w", cmd.CLI, hostname, err)
		}
		allOutput = append(allOutput, []byte(cmd.CLI+"\n")...)
		allOutput = append(allOutput, output...)
		allOutput = append(allOutput, '\n')
	}

	return allOutput, nil
}
