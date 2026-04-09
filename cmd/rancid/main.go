package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"path/filepath"
	"strings"

	"gorancid/pkg/config"
	"gorancid/pkg/connect"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/parse"

	// Register device parsers
	_ "gorancid/pkg/parse/fortigate"
	_ "gorancid/pkg/parse/ios"
	_ "gorancid/pkg/parse/iosxr"
	_ "gorancid/pkg/parse/junos"
	_ "gorancid/pkg/parse/nxos"
)

const version = "0.2.0"

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

	// Check if a Go parser is available for this device type
	goParserAvailable := false
	if _, ok := parse.Lookup(*deviceType); ok {
		goParserAvailable = true
	}

	if goParserAvailable {
		// Use Go-native SSH connection and parser
		collectWithGoParser(hostname, creds, spec, filterOpts, outFile, cfg)
	} else {
		// Fall back to Expect subprocess + Perl rancid
		collectWithFallback(hostname, spec, outFile)
	}
}

// collectWithGoParser connects via SSH, runs commands, and parses output in Go.
func collectWithGoParser(hostname string, creds config.Credentials, spec devicetype.DeviceSpec, filterOpts parse.FilterOpts, outFile string, cfg config.Config) {
	parser, _ := parse.Lookup(spec.Type)

	// Get device-specific connection options from the parser if available
	opts := connect.DeviceOpts{
		DeviceType:      spec.Type,
		SetupCommands:   []string{"terminal length 0"},
		DisablePagingCmd: "terminal length 0",
		Timeout:         30 * time.Second,
	}
	// Try to get DeviceOpts from the parser if it implements DeviceOptsProvider
	if provider, ok := parser.(interface{ DeviceOpts() connect.DeviceOpts }); ok {
		opts = provider.DeviceOpts()
	}

	// Determine SSH port from methods
	port := 22
	for _, m := range creds.Methods {
		if m == "telnet" {
			port = 23
		}
	}

	// Connect
	session := &connect.SSHSession{
		Host:  hostname,
		Port:  port,
		Creds: creds,
		Opts:  opts,
	}
	ctx := context.Background()

	if err := session.Connect(ctx); err != nil {
		log.Fatalf("connect %s: %v", hostname, err)
	}
	defer session.Close()

	// Run each command from the device spec
	var allOutput []byte
	for _, cmd := range spec.Commands {
		output, err := session.RunCommand(ctx, cmd.CLI)
		if err != nil {
			log.Printf("command %q on %s: %v", cmd.CLI, hostname, err)
			continue
		}
		allOutput = append(allOutput, output...)
		allOutput = append(allOutput, '\n')
	}

	// Parse the collected output
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

// collectWithFallback shells out to the original Perl rancid script.
func collectWithFallback(hostname string, spec devicetype.DeviceSpec, outFile string) {
	bin := spec.Script
	if bin == "" {
		bin = fmt.Sprintf("rancid -t %s", spec.Type)
	}

	// Parse the script field — it's "rancid -t <type>"
	parts := strings.Fields(bin)
	args := append(parts[1:], hostname)

	cmd := context.Background()
	result, err := connect.RunExpectCommand(cmd, parts[0], args, "")
	if err != nil {
		log.Fatalf("fallback %s: %v", hostname, err)
	}

	if err := os.WriteFile(outFile, result, 0644); err != nil {
		log.Fatalf("write %s: %v", outFile, err)
	}
}