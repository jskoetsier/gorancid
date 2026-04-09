package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gorancid/pkg/collect"
	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/git"
	"gorancid/pkg/notify"
	"gorancid/pkg/par"
)

const version = "0.1.0"

func main() {
	var (
		showVersion = flag.Bool("V", false, "print version")
		commitMsg   = flag.String("c", "", "VCS commit message override")
		cfgFile     = flag.String("f", "", "group config file (router.db path)")
		onlyDevice  = flag.String("r", "", "collect only this device hostname")
		mailRcpt    = flag.String("m", "", "mail recipients override")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("control-rancid %s\n", version)
		os.Exit(0)
	}
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: control-rancid [-V] [-c msg] [-f router.db] [-r hostname] [-m mail] <group>")
		os.Exit(1)
	}
	group := flag.Arg(0)

	// Load rancid.conf
	confPath := os.Getenv("RANCID_CONF")
	if confPath == "" {
		confPath = "/usr/local/rancid/etc/rancid.conf"
	}
	cfg, err := config.Load(confPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Load router.db
	rdbPath := filepath.Join(cfg.BaseDir, group, "router.db")
	if *cfgFile != "" {
		rdbPath = *cfgFile
	}
	devices, err := config.LoadRouterDB(rdbPath)
	if err != nil {
		log.Fatalf("router.db: %v", err)
	}

	// Load .cloginrc
	cloginPath := filepath.Join(os.Getenv("HOME"), ".cloginrc")
	credStore, err := config.LoadCloginrc(cloginPath)
	if err != nil {
		log.Printf("warning: cloginrc: %v", err)
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

	outDir := filepath.Join(cfg.BaseDir, group, "configs")

	// Build parallel jobs — one per active device
	type jobMeta struct{ hostname string }
	var jobs []par.Job
	var meta []jobMeta

	for _, dev := range devices {
		if dev.Status != "up" {
			continue
		}
		if *onlyDevice != "" && dev.Hostname != *onlyDevice {
			continue
		}
		spec, ok := devicetype.Lookup(typeSpecs, dev.Type)
		if !ok {
			log.Printf("warning: unknown device type %q for %s — skipping", dev.Type, dev.Hostname)
			continue
		}
		var creds config.Credentials
		if credStore != nil {
			creds = credStore.Lookup(dev.Hostname)
		}
		d, s, c := dev, spec, creds // capture loop vars
		fc := &collect.FallbackCollector{
			Device: d,
			Spec:   s,
			Creds:  c,
			OutDir: outDir,
		}
		jobs = append(jobs, func(ctx context.Context) error {
			result, err := fc.Run(ctx)
			if err != nil {
				return err
			}
			if result.Error != nil {
				log.Printf("collect %s: %v", result.Hostname, result.Error)
			}
			return nil
		})
		meta = append(meta, jobMeta{dev.Hostname})
	}

	if len(jobs) == 0 {
		log.Println("no devices to collect")
		os.Exit(0)
	}

	results := par.Run(context.Background(), jobs, cfg.ParCount)

	// Gather successful hosts for commit
	var changed []string
	for i, r := range results {
		if r.Err == nil {
			changed = append(changed, meta[i].hostname)
		}
	}

	if len(changed) == 0 {
		log.Println("no successful collections")
		os.Exit(0)
	}

	// Stage and commit
	repoDir := filepath.Join(cfg.BaseDir, group)
	var stageFiles []string
	for _, h := range changed {
		stageFiles = append(stageFiles, filepath.Join("configs", h))
	}
	if err := git.Add(repoDir, stageFiles); err != nil {
		log.Printf("git add: %v", err)
	}

	msg := fmt.Sprintf("rancid collection for group %s", group)
	if *commitMsg != "" {
		msg = *commitMsg
	}
	if err := git.Commit(repoDir, msg); err != nil {
		log.Printf("git commit: %v (possibly nothing to commit)", err)
	}

	// Get diff and notify
	diff, _ := git.Diff(repoDir, "configs/")
	if len(diff) > 0 {
		rcpts := []string{fmt.Sprintf("rancid-%s", group)}
		if *mailRcpt != "" {
			rcpts = []string{*mailRcpt}
		}
		notifyCfg := notify.Config{
			SendMail:    cfg.SendMail,
			Recipients:  rcpts,
			Subject:     fmt.Sprintf("rancid diffs for %s", group),
			MailDomain:  cfg.MailDomain,
			MailHeaders: cfg.MailHeaders,
			MailOpts:    cfg.MailOpts,
		}
		if err := notify.SendDiff(notifyCfg, diff); err != nil {
			log.Printf("notify: %v", err)
		}
	}
}