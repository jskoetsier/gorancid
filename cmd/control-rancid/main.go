package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gorancid/pkg/collect"
	"gorancid/pkg/config"
	"gorancid/pkg/connect"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/git"
	"gorancid/pkg/notify"
	"gorancid/pkg/par"
	"gorancid/pkg/parse"
	"gorancid/pkg/version"
)

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
		fmt.Printf("control-rancid %s\n", version.Version)
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
	devicetype.RegisterMissingParsers(typeSpecs)

	outDir := filepath.Join(cfg.BaseDir, group, "configs")

	// Build parallel jobs — one per active device
	type jobMeta struct{ hostname string }
	var jobs []par.Job
	var meta []jobMeta

	selected, selSpecs, selCreds, skipped := selectDevices(devices, typeSpecs, credStore, *onlyDevice)
	for _, h := range skipped {
		log.Printf("warning: %s", h)
	}
	filterOpts := parse.FilterOpts{
		FilterPwds: int(cfg.FilterPwds),
		FilterOsc:  int(cfg.FilterOsc),
		NoCommStr:  cfg.NoCommStr,
	}
	for i := range selected {
		d, s, c := selected[i], selSpecs[i], selCreds[i]
		gc := &collect.GoCollector{
			Device:     d,
			Spec:       s,
			Creds:      c,
			OutDir:     outDir,
			FilterOpts: filterOpts,
		}
		jobs = append(jobs, func(ctx context.Context) error {
			result, err := gc.Run(ctx)
			if err != nil {
				return err
			}
			if result.Error != nil {
				if errors.Is(result.Error, connect.ErrNoNativeTransport) {
					log.Printf("collect %s: %v — add an ssh or telnet method for this host in %s (example: add method * { ssh } or { telnet })", result.Hostname, result.Error, cloginPath)
				} else {
					log.Printf("collect %s: %v", result.Hostname, result.Error)
				}
			}
			return nil
		})
		meta = append(meta, jobMeta{selected[i].Hostname})
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

// selectDevices filters the router.db entries and returns the devices that should
// be collected, along with their resolved specs, credentials, and a list of
// skip-reason strings for logging.
func selectDevices(
	devices []config.Device,
	typeSpecs map[string]devicetype.DeviceSpec,
	credStore *config.CredStore,
	onlyDevice string,
) ([]config.Device, []devicetype.DeviceSpec, []config.Credentials, []string) {
	var (
		selected []config.Device
		specs    []devicetype.DeviceSpec
		creds    []config.Credentials
		skipped  []string
	)
	for _, dev := range devices {
		if dev.Status != "up" {
			continue
		}
		if onlyDevice != "" && dev.Hostname != onlyDevice {
			continue
		}
		spec, ok := devicetype.Lookup(typeSpecs, dev.Type)
		if !ok {
			skipped = append(skipped, fmt.Sprintf("unknown device type %q for %s — skipping", dev.Type, dev.Hostname))
			continue
		}
		var c config.Credentials
		if credStore != nil {
			c = credStore.Lookup(dev.Hostname)
		}
		selected = append(selected, dev)
		specs = append(specs, spec)
		creds = append(creds, c)
	}
	return selected, specs, creds, skipped
}
