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

const version = "0.3.0"

func main() {
	var (
		showVersion = flag.Bool("V", false, "print version")
		showHelp    = flag.Bool("h", false, "show usage")
		autoEnable  = flag.Bool("autoenable", false, "assume device is already enabled")
		noEnable    = flag.Bool("noenable", false, "do not enter enable mode")
		commandStr  = flag.String("c", "", "commands to run, separated by semicolons")
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
	confPath := os.Getenv("RANCID_CONF")
	if confPath == "" {
		confPath = "/usr/local/rancid/etc/rancid.conf"
	}
	cloginPath := *cloginrc
	if cloginPath == "" {
		cloginPath = filepath.Join(os.Getenv("HOME"), ".cloginrc")
	}
	sysconfdir := os.Getenv("RANCID_SYSCONFDIR")
	if sysconfdir == "" {
		sysconfdir = "/usr/local/rancid/etc"
	}

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
		if err := runNative(context.Background(), hostname, spec.Type, creds, commands, timeout, *noEnable, *autoEnable, *interactive || len(commands) == 0); err != nil {
			log.Fatalf("clogin: %v", err)
		}
		return
	}

	if err := runLegacy(context.Background(), spec.LoginScript, hostname, cloginPath, commands, creds, *timeoutSec, *noEnable, *autoEnable, *interactive); err != nil {
		log.Fatalf("clogin fallback: %v", err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: clogin [-Vh] [-autoenable] [-noenable] [-i] [-c command] [-e enable-password] [-f cloginrc-file] [-p user-password] [-t timeout] [-u username] [-z device_type] [-routerdb path] hostname")
	os.Exit(1)
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
	if _, ok := parse.Lookup(deviceType); !ok {
		return false
	}
	_, ok := firstSSHMethod(methods)
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

func runLegacy(ctx context.Context, loginScript, hostname, cloginrc string, commands []string, creds config.Credentials, timeoutSec int, noEnable, autoEnable, interactive bool) error {
	bin := loginScript
	if bin == "" {
		bin = "clogin"
	}

	args := []string{}
	if cloginrc != "" {
		args = append(args, "-f", cloginrc)
	}
	if creds.Username != "" {
		args = append(args, "-u", creds.Username)
	}
	if creds.Password != "" {
		args = append(args, "-p", creds.Password)
	}
	if creds.EnablePwd != "" {
		args = append(args, "-e", creds.EnablePwd)
	}
	if timeoutSec > 0 {
		args = append(args, "-t", strconv.Itoa(timeoutSec))
	}
	if noEnable {
		args = append(args, "-noenable")
	}
	if autoEnable {
		args = append(args, "-autoenable")
	}
	if len(commands) > 0 {
		args = append(args, "-c", strings.Join(commands, "; "))
	}
	if interactive {
		args = append(args, "-i")
	}
	args = append(args, hostname)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
