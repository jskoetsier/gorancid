package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorancid/pkg/config"
	"gorancid/pkg/connect"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/parse"

	_ "gorancid/pkg/parse/fortigate"
	_ "gorancid/pkg/parse/ios"
	_ "gorancid/pkg/parse/iosxr"
	_ "gorancid/pkg/parse/junos"
	_ "gorancid/pkg/parse/nxos"
)

const version = "0.3.1"

func main() {
	var (
		showVersion = flag.Bool("V", false, "print version")
		showHelp    = flag.Bool("h", false, "show usage")
		autoEnable  = flag.Bool("autoenable", false, "assume device is already enabled")
		noEnable    = flag.Bool("noenable", false, "do not enter enable mode")
		commandStr  = flag.String("c", "", "commands to run, separated by semicolons")
		confFile    = flag.String("C", "", "rancid.conf file path")
		confFileAlt = flag.String("config", "", "rancid.conf file path")
		cloginrc    = flag.String("f", "", "cloginrc file path")
		enablePwd   = flag.String("e", "", "enable password override")
		password    = flag.String("p", "", "user password override")
		timeoutSec  = flag.Int("t", 30, "command timeout in seconds")
		username    = flag.String("u", "", "username override")
		interactive = flag.Bool("i", false, "stay interactive after -c commands")
		deviceType  = flag.String("z", "", "device type override")
		routerDB    = flag.String("routerdb", "", "router.db path override")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("clogin %s\n", version)
		os.Exit(0)
	}
	if *showHelp || flag.NArg() != 1 {
		usage()
	}

	hostname := flag.Arg(0)
	confPath := resolveConfigPath(*confFile, *confFileAlt)
	cloginPath := *cloginrc
	if cloginPath == "" {
		cloginPath = filepath.Join(os.Getenv("HOME"), ".cloginrc")
	}
	sysconfdir := resolveSysconfDir(confPath)

	cfg, err := config.Load(confPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	credStore, err := config.LoadCloginrc(cloginPath)
	if err != nil {
		log.Fatalf("cloginrc: %v", err)
	}
	specs, err := devicetype.Load(
		filepath.Join(sysconfdir, "rancid.types.base"),
		filepath.Join(sysconfdir, "rancid.types.conf"),
	)
	if err != nil {
		log.Fatalf("devicetype: %v", err)
	}
	ensureParserCoverage(specs)

	resolvedType := *deviceType
	if resolvedType == "" {
		dev, _, err := findDevice(hostname, cfg, *routerDB)
		if err != nil {
			log.Fatalf("router.db lookup: %v", err)
		}
		resolvedType = dev.Type
	}

	spec, ok := devicetype.Lookup(specs, resolvedType)
	if !ok {
		log.Fatalf("unknown device type: %s", resolvedType)
	}

	creds := credStore.Lookup(hostname)
	if *username != "" {
		creds.Username = *username
	}
	if *password != "" {
		creds.Password = *password
	}
	if *enablePwd != "" {
		creds.EnablePwd = *enablePwd
	}
	if len(creds.Methods) == 0 {
		creds.Methods = []string{"ssh"}
	}

	timeout := time.Duration(*timeoutSec) * time.Second
	commands := splitCommands(*commandStr)
	if canUseNative(spec.Type, creds.Methods) {
		fmt.Fprintf(os.Stderr, "using native ssh: type=%s host=%s\n", spec.Type, hostname)
		if err := runNative(context.Background(), hostname, spec.Type, creds, commands, timeout, *noEnable, *autoEnable, *interactive || len(commands) == 0); err != nil {
			log.Fatalf("clogin: %v", err)
		}
		return
	}

	legacyScript := resolveLegacyLoginScript(spec)
	fmt.Fprintf(os.Stderr, "falling back to legacy login script: script=%s type=%s host=%s\n", legacyScript, spec.Type, hostname)
	if err := runLegacy(
		context.Background(),
		legacyScript,
		hostname,
		cloginPath,
		commands,
		*timeoutSec,
		*noEnable,
		*autoEnable,
		*interactive,
		*username,
		*password,
		*enablePwd,
	); err != nil {
		log.Fatalf("clogin fallback: %v", err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: clogin [-Vh] [-autoenable] [-noenable] [-i] [-C rancid.conf] [-config rancid.conf] [-c command] [-e enable-password] [-f cloginrc-file] [-p user-password] [-t timeout] [-u username] [-z device_type] [-routerdb path] hostname")
	os.Exit(1)
}

func resolveConfigPath(primary, secondary string) string {
	if primary != "" {
		return primary
	}
	if secondary != "" {
		return secondary
	}
	if confPath := os.Getenv("RANCID_CONF"); confPath != "" {
		return confPath
	}
	return "/usr/local/rancid/etc/rancid.conf"
}

func resolveSysconfDir(confPath string) string {
	if sysconfdir := os.Getenv("RANCID_SYSCONFDIR"); sysconfdir != "" {
		return sysconfdir
	}
	if confPath != "" {
		return filepath.Dir(confPath)
	}
	return "/usr/local/rancid/etc"
}

func findDevice(hostname string, cfg config.Config, routerDBPath string) (config.Device, string, error) {
	if routerDBPath != "" {
		devices, err := config.LoadRouterDB(routerDBPath)
		if err != nil {
			return config.Device{}, "", err
		}
		for _, dev := range devices {
			if dev.Hostname == hostname {
				return dev, filepath.Base(filepath.Dir(routerDBPath)), nil
			}
		}
		return config.Device{}, "", fmt.Errorf("%s not found in %s", hostname, routerDBPath)
	}

	for _, group := range cfg.Groups {
		path := filepath.Join(cfg.BaseDir, group, "router.db")
		devices, err := config.LoadRouterDB(path)
		if err != nil {
			continue
		}
		for _, dev := range devices {
			if dev.Hostname == hostname {
				return dev, group, nil
			}
		}
	}
	return config.Device{}, "", fmt.Errorf("%s not found in configured router.dbs", hostname)
}

func splitCommands(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	commands := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			commands = append(commands, part)
		}
	}
	return commands
}

func canUseNative(deviceType string, methods []string) bool {
	parser, ok := parse.Lookup(deviceType)
	if !ok {
		return false
	}
	if _, ok := parser.(interface{ DeviceOpts() connect.DeviceOpts }); !ok {
		return false
	}
	_, ok = firstSSHMethod(methods)
	return ok
}

func firstSSHMethod(methods []string) (int, bool) {
	if len(methods) == 0 {
		return 22, true
	}
	for _, method := range methods {
		if method == "ssh" {
			return 22, true
		}
		if strings.HasPrefix(method, "ssh:") {
			port, err := strconv.Atoi(strings.TrimPrefix(method, "ssh:"))
			if err == nil && port > 0 {
				return port, true
			}
		}
	}
	return 0, false
}

func runNative(ctx context.Context, hostname, deviceType string, creds config.Credentials, commands []string, timeout time.Duration, noEnable, autoEnable, interactive bool) error {
	port, ok := firstSSHMethod(creds.Methods)
	if !ok {
		return fmt.Errorf("native mode requires an ssh method in .cloginrc")
	}

	opts := deviceOpts(deviceType, creds, timeout, noEnable, autoEnable)
	session := &connect.SSHSession{
		Host:  hostname,
		Port:  port,
		Creds: creds,
		Opts:  opts,
	}
	if err := session.Connect(ctx); err != nil {
		return err
	}
	defer session.Close()

	for _, cmd := range commands {
		output, err := session.RunCommand(ctx, cmd)
		if len(output) > 0 {
			if _, werr := os.Stdout.Write(output); werr != nil {
				return werr
			}
			if output[len(output)-1] != '\n' {
				fmt.Fprintln(os.Stdout)
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", cmd, err)
		}
	}

	if interactive {
		return session.Interact(ctx, os.Stdin, os.Stdout)
	}
	return nil
}

func deviceOpts(deviceType string, creds config.Credentials, timeout time.Duration, noEnable, autoEnable bool) connect.DeviceOpts {
	opts := connect.DeviceOpts{
		DeviceType: deviceType,
		Timeout:    timeout,
	}
	if parser, ok := parse.Lookup(deviceType); ok {
		if provider, ok := parser.(interface{ DeviceOpts() connect.DeviceOpts }); ok {
			opts = provider.DeviceOpts()
			opts.Timeout = timeout
		}
	}
	if noEnable || autoEnable {
		opts.EnableCmd = ""
		return opts
	}
	if opts.EnableCmd == "" && creds.EnablePwd != "" && wantsEnable(deviceType) {
		opts.EnableCmd = "enable"
	}
	return opts
}

func wantsEnable(deviceType string) bool {
	switch strings.ToLower(deviceType) {
	case "ios", "cisco", "cat5k", "nxos":
		return true
	default:
		return false
	}
}

func resolveLegacyLoginScript(spec devicetype.DeviceSpec) string {
	if spec.LoginScript != "" {
		return spec.LoginScript
	}
	for _, module := range spec.Modules {
		switch strings.ToLower(module) {
		case "fortigate":
			return "fnlogin"
		case "junos":
			return "jlogin"
		case "ios", "iosxr", "nxos":
			return "clogin"
		}
	}
	switch {
	case strings.HasPrefix(strings.ToLower(spec.Type), "forti"):
		return "fnlogin"
	case strings.HasPrefix(strings.ToLower(spec.Type), "jun"):
		return "jlogin"
	default:
		return "clogin"
	}
}

func runLegacy(ctx context.Context, loginScript, hostname, cloginrc string, commands []string, timeoutSec int, noEnable, autoEnable, interactive bool, username, password, enablePwd string) error {
	bin := loginScript
	if bin == "" {
		bin = "clogin"
	}

	args := []string{}
	if cloginrc != "" {
		args = append(args, "-f", cloginrc)
	}
	if username != "" && scriptSupports(bin, "username") {
		args = append(args, "-u", username)
	}
	if password != "" && scriptSupports(bin, "password") {
		args = append(args, "-p", password)
	}
	if enablePwd != "" && scriptSupports(bin, "enable-password") {
		args = append(args, "-e", enablePwd)
	}
	if timeoutSec > 0 {
		args = append(args, "-t", strconv.Itoa(timeoutSec))
	}
	if noEnable && scriptSupports(bin, "noenable") {
		args = append(args, "-noenable")
	}
	if autoEnable && scriptSupports(bin, "autoenable") {
		args = append(args, "-autoenable")
	}
	if len(commands) > 0 {
		args = append(args, "-c", strings.Join(commands, "; "))
	}
	if interactive && scriptSupports(bin, "interactive") {
		args = append(args, "-i")
	}
	args = append(args, hostname)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func scriptSupports(script, capability string) bool {
	switch strings.ToLower(script) {
	case "clogin":
		return true
	case "fnlogin", "jlogin":
		switch capability {
		case "username", "password":
			return true
		default:
			return false
		}
	default:
		switch capability {
		case "username", "password":
			return true
		default:
			return false
		}
	}
}
