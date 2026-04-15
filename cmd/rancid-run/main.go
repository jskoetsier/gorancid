package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gorancid/pkg/config"
)

const version = "0.3.6"

func main() {
	var (
		showVersion = flag.Bool("V", false, "print version")
		confFile    = flag.String("f", "", "rancid.conf path override")
		mailRcpt    = flag.String("m", "", "mail recipients override (passed to control-rancid)")
		onlyDevice  = flag.String("r", "", "collect only this device (passed to control-rancid)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("rancid-run %s\n", version)
		os.Exit(0)
	}

	confPath := *confFile
	if confPath == "" {
		confPath = os.Getenv("RANCID_CONF")
	}
	if confPath == "" {
		confPath = "/usr/local/rancid/etc/rancid.conf"
	}

	cfg, err := config.Load(confPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	groups := flag.Args()
	if len(groups) == 0 {
		groups = cfg.Groups
	}
	if len(groups) == 0 {
		log.Fatal("no groups to run — set LIST_OF_GROUPS in rancid.conf or pass groups on command line")
	}

	logDir := cfg.LogDir
	if logDir == "" {
		logDir = filepath.Join(cfg.BaseDir, "logs")
	}
	if err := os.MkdirAll(logDir, 0750); err != nil {
		log.Fatalf("mkdir %s: %v", logDir, err)
	}

	timestamp := time.Now().Format("2006-01-02T15:04:05")
	exitCode := 0

	for _, group := range groups {
		logFile := filepath.Join(logDir, group+"."+timestamp)
		lf, err := os.Create(logFile)
		if err != nil {
			log.Printf("cannot create log file %s: %v", logFile, err)
			lf = os.Stderr
		} else {
			defer lf.Close()
		}

		args := []string{group}
		if *mailRcpt != "" {
			args = append([]string{"-m", *mailRcpt}, args...)
		}
		if *onlyDevice != "" {
			args = append([]string{"-r", *onlyDevice}, args...)
		}

		cmd := exec.Command("control-rancid", args...)
		cmd.Stdout = lf
		cmd.Stderr = lf
		cmd.Env = append(os.Environ(), "RANCID_CONF="+confPath)

		fmt.Fprintf(lf, "Starting rancid collection for group %s at %s\n", group, timestamp)
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(lf, "control-rancid %s failed: %v\n", group, err)
			exitCode = 1
		} else {
			fmt.Fprintf(lf, "Completed group %s\n", group)
		}
	}

	os.Exit(exitCode)
}
