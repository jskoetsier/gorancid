package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gorancid/pkg/config"
	"gorancid/pkg/git"
)

const version = "0.4.1"

func main() {
	showVersion := flag.Bool("V", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("rancid-cvs %s\n", version)
		os.Exit(0)
	}

	confPath := os.Getenv("RANCID_CONF")
	if confPath == "" {
		confPath = "/usr/local/rancid/etc/rancid.conf"
	}

	cfg, err := config.Load(confPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if cfg.BaseDir == "" {
		log.Fatal("BASEDIR not set in rancid.conf")
	}
	if len(cfg.Groups) == 0 {
		log.Fatal("LIST_OF_GROUPS not set in rancid.conf")
	}

	// Initialize one git repo per group under BASEDIR.
	for _, group := range cfg.Groups {
		groupDir := filepath.Join(cfg.BaseDir, group)
		configsDir := filepath.Join(groupDir, "configs")

		for _, dir := range []string{groupDir, configsDir} {
			if err := os.MkdirAll(dir, 0750); err != nil {
				log.Fatalf("mkdir %s: %v", dir, err)
			}
		}

		repoDir := groupDir
		if err := git.Init(repoDir); err != nil {
			log.Fatalf("git init %s: %v", repoDir, err)
		}

		// Create .gitignore to exclude lock files
		gitignore := filepath.Join(repoDir, ".gitignore")
		if _, err := os.Stat(gitignore); os.IsNotExist(err) {
			_ = os.WriteFile(gitignore, []byte(".lock\n*.new\n"), 0640)
		}

		fmt.Printf("Initialized group %s at %s\n", group, groupDir)
	}

	// Create logs directory
	logsDir := cfg.LogDir
	if logsDir == "" {
		logsDir = filepath.Join(cfg.BaseDir, "logs")
	}
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		log.Fatalf("mkdir %s: %v", logsDir, err)
	}
	fmt.Printf("Log directory: %s\n", logsDir)
}
