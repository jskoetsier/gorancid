package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorancid/pkg/collect"
	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/parse"
	"gorancid/pkg/version"
)

func main() {
	var (
		showVersion = flag.Bool("V", false, "print version")
		deviceType  = flag.String("t", "", "device type (required)")
		outputFile  = flag.String("f", "", "output file (defaults to <hostname>.new in CWD)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("gorancid %s\n", version.Version)
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

	confPath := os.Getenv("RANCID_CONF")
	if confPath == "" {
		confPath = "/usr/local/rancid/etc/rancid.conf"
	}
	cfg, err := config.Load(confPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	cloginPath := filepath.Join(os.Getenv("HOME"), ".cloginrc")
	credStore, err := config.LoadCloginrc(cloginPath)
	if err != nil {
		log.Printf("warning: cloginrc: %v", err)
	}
	var creds config.Credentials
	if credStore != nil {
		creds = credStore.Lookup(hostname)
	}

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
	devicetype.RegisterMissingParsers(typeSpecs)

	spec, ok := devicetype.Lookup(typeSpecs, *deviceType)
	if !ok {
		log.Fatalf("unknown device type: %s", *deviceType)
	}

	filterOpts := parse.FilterOpts{
		FilterPwds: int(cfg.FilterPwds),
		FilterOsc:  int(cfg.FilterOsc),
		NoCommStr:  cfg.NoCommStr,
	}

	outFile := *outputFile
	if outFile == "" {
		outFile = hostname + ".new"
	}

	timeout := 30 * time.Second
	if spec.Timeout > 0 {
		timeout = spec.Timeout
	}

	ctx := context.Background()
	if err := collect.CollectDevice(ctx, hostname, creds, spec, filterOpts, outFile, timeout); err != nil {
		log.Fatalf("collect %s: %v", hostname, err)
	}
}
