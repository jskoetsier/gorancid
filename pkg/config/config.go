package config

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// FilterMode represents FILTER_PWDS / FILTER_OSC values.
type FilterMode int

const (
	FilterNo  FilterMode = 0
	FilterYes FilterMode = 1
	FilterAll FilterMode = 2
)

// Config holds settings parsed from rancid.conf.
type Config struct {
	BaseDir     string
	LogDir      string
	RepoRoot    string // CVSROOT env var — used as git repo base path
	SendMail    string
	Groups      []string
	FilterPwds  FilterMode
	FilterOsc   FilterMode
	NoCommStr   bool
	ParCount    int
	OldTime     int
	LockTime    int
	MaxRounds   int
	MailDomain  string
	MailOpts    string
	MailSplit   int
	MailHeaders string
}

// assignRE matches KEY=value lines, ignoring trailing ; export KEY and comments.
var assignRE = regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)=([^;#\n]*)`)

// Load reads rancid.conf from path and returns a populated Config.
// Only KEY=value assignment lines are parsed; shell logic is ignored.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := assignRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := m[1]
		val := strings.Trim(strings.TrimSpace(m[2]), `"'`)
		env[key] = val
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		BaseDir:     env["BASEDIR"],
		LogDir:      env["LOGDIR"],
		RepoRoot:    env["CVSROOT"],
		SendMail:    env["SENDMAIL"],
		MailDomain:  env["MAILDOMAIN"],
		MailOpts:    env["MAILOPTS"],
		MailHeaders: env["MAILHEADERS"],
	}
	if g := env["LIST_OF_GROUPS"]; g != "" {
		cfg.Groups = strings.Fields(g)
	}

	cfg.ParCount = intOr(env["PAR_COUNT"], 5)
	cfg.OldTime = intOr(env["OLDTIME"], 24)
	cfg.LockTime = intOr(env["LOCKTIME"], 4)
	cfg.MaxRounds = intOr(env["MAX_ROUNDS"], 4)
	cfg.MailSplit = intOr(env["MAILSPLIT"], 0)

	cfg.FilterPwds = parseFilterMode(env["FILTER_PWDS"])
	cfg.FilterOsc = parseFilterMode(env["FILTER_OSC"])

	if strings.EqualFold(env["NOCOMMSTR"], "yes") {
		cfg.NoCommStr = true
	}
	return cfg, nil
}

func parseFilterMode(s string) FilterMode {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "NO":
		return FilterNo
	case "ALL":
		return FilterAll
	default:
		return FilterYes
	}
}

func intOr(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}
