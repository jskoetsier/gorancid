package main

import (
	"context"
	"flag"
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

	// Register device parsers
	_ "gorancid/pkg/parse/fortigate"
	_ "gorancid/pkg/parse/generic"
	_ "gorancid/pkg/parse/ios"
	_ "gorancid/pkg/parse/iosxr"
	_ "gorancid/pkg/parse/junos"
	_ "gorancid/pkg/parse/nxos"
)

const version = "0.3.1"

func main() {
	var (
		showVersion = flag.Bool("V", false, "print version")
		deviceType  = flag.String("t", "", "device type (required)")
		outputFile  = flag.String("f", "", "output file (defaults to <hostname>.new in CWD)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("gorancid %s\n", version)
		os.Exit(0)
	}

	if *deviceType == "" {
		fmt.Fprintln(os.Stderr, "usage: rancid -t device_type [-f filename] hostname")
		os.Exit(1)
	}
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: rancid -t device_type [-f filename] hostname")
		os.Exit(1)
	}
	hostname := flag.Arg(0)

	// Load configuration
	confPath := os.Getenv("RANCID_CONF")
	if confPath == "" {
		confPath = "/usr/local/rancid/etc/rancid.conf"
	}
	cfg, err := config.Load(confPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Load credentials
	cloginPath := filepath.Join(os.Getenv("HOME"), ".cloginrc")
	credStore, err := config.LoadCloginrc(cloginPath)
	if err != nil {
		log.Printf("warning: cloginrc: %v", err)
	}
	var creds config.Credentials
	if credStore != nil {
		creds = credStore.Lookup(hostname)
	}

	// Load device type specs
	sysconfdir := os.Getenv("RANCID_SYSCONFDIR")
	if sysconfdir == "" {
		sysconfdir = "/usr/local/rancid/etc"
	}
	typeSpecs, err := devicetype.Load(
		filepath.Join(sysconfdir, "rancid.types.base"),
		filepath.Join(sysconfdir, "rancid.types.conf"),
	)
	if err != nil {
		log.Fatalf("devicetype: %v", err)
	}
	ensureParserCoverage(typeSpecs)

	spec, ok := devicetype.Lookup(typeSpecs, *deviceType)
	if !ok {
		log.Fatalf("unknown device type: %s", *deviceType)
	}

	// Build filter options from config
	filterOpts := parse.FilterOpts{
		FilterPwds: int(cfg.FilterPwds),
		FilterOsc:  int(cfg.FilterOsc),
		NoCommStr:  cfg.NoCommStr,
	}

	// Determine output file
	outFile := *outputFile
	if outFile == "" {
		outFile = hostname + ".new"
	}

	collectWithGoParser(hostname, creds, spec, filterOpts, outFile, cfg)
}

type deviceOptsProvider interface {
	DeviceOpts() connect.DeviceOpts
}

type bulkRunner interface {
	RunAll(ctx context.Context, commands []string) ([]byte, error)
}

// collectWithGoParser connects via SSH or Expect transport, runs commands, and parses output in Go.
func collectWithGoParser(hostname string, creds config.Credentials, spec devicetype.DeviceSpec, filterOpts parse.FilterOpts, outFile string, cfg config.Config) {
	parser, _ := parse.Lookup(spec.Type)

	opts := connect.DeviceOpts{
		DeviceType: spec.Type,
		Timeout:    30 * time.Second,
	}
	preferNative := false
	if provider, ok := parser.(deviceOptsProvider); ok {
		opts = provider.DeviceOpts()
		if opts.Timeout == 0 {
			opts.Timeout = 30 * time.Second
		}
		preferNative = true
	}

	ctx := context.Background()
	session := connect.NewSession(hostname, 22, creds, opts, spec.LoginScript, preferNative)
	if err := session.Connect(ctx); err != nil {
		log.Fatalf("connect %s: %v", hostname, err)
	}
	defer session.Close()

	commandList := make([]string, 0, len(spec.Commands))
	for _, cmd := range spec.Commands {
		commandList = append(commandList, cmd.CLI)
	}
	allOutput, err := collectOutput(ctx, session, commandList, hostname)
	if err != nil {
		log.Fatalf("collect %s: %v", hostname, err)
	}

	parsed, err := parser.Parse(allOutput, filterOpts)
	if err != nil {
		log.Fatalf("parse %s: %v", hostname, err)
	}

	// Write output
	output := strings.Join(parsed.Lines, "\n")
	if err := os.WriteFile(outFile, []byte(output), 0644); err != nil {
		log.Fatalf("write %s: %v", outFile, err)
	}

	// Also write metadata if present
	if len(parsed.Metadata) > 0 {
		metaFile := strings.TrimSuffix(outFile, ".new") + ".meta"
		var metaLines []string
		for k, v := range parsed.Metadata {
			metaLines = append(metaLines, fmt.Sprintf("%s: %s", k, v))
		}
		if err := os.WriteFile(metaFile, []byte(strings.Join(metaLines, "\n")+"\n"), 0644); err != nil {
			log.Printf("write metadata %s: %v", metaFile, err)
		}
	}
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
