// Command rancid-ui is a read-only local web UI for browsing collected configs,
// recent per-device diffs, and fleet status derived from router.db and git.
package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"gorancid/pkg/config"
	"gorancid/pkg/git"
)

//go:embed templates/*.html
var templateFS embed.FS

const version = "0.4.0"

var hostPat = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func main() {
	var (
		showVer = flag.Bool("V", false, "print version")
		listen  = flag.String("listen", "127.0.0.1:8080", "HTTP listen address (default loopback)")
		conf    = flag.String("C", "", "path to rancid.conf (or set RANCID_CONF)")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("rancid-ui %s\n", version)
		os.Exit(0)
	}

	confPath := *conf
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

	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	s := &server{cfg: cfg, tmpl: tmpl}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.handleFleet(w, r)
	})
	mux.Handle("GET /group/{group}", http.HandlerFunc(s.handleGroup))
	mux.Handle("GET /group/{group}/device/{host}/config", http.HandlerFunc(s.handleConfig))
	mux.Handle("GET /group/{group}/device/{host}/diff", http.HandlerFunc(s.handleDiff))

	log.Printf("rancid-ui %s listening on http://%s/ (rancid.conf=%s)", version, *listen, confPath)
	srv := &http.Server{
		Addr:              *listen,
		Handler:           withSecurityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

type server struct {
	cfg  config.Config
	tmpl *template.Template
}

func (s *server) allowedGroup(name string) bool {
	for _, g := range s.cfg.Groups {
		if g == name {
			return true
		}
	}
	return false
}

func (s *server) handleFleet(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Group, Host, DevType, Status string
	}
	var rows []row
	for _, g := range s.cfg.Groups {
		devs, err := config.LoadRouterDB(filepath.Join(s.cfg.BaseDir, g, "router.db"))
		if err != nil {
			continue
		}
		for _, d := range devs {
			rows = append(rows, row{Group: g, Host: d.Hostname, DevType: d.Type, Status: d.Status})
		}
	}
	s.render(w, "fleet.html", map[string]any{
		"Title": "Fleet",
		"Rows":  rows,
	})
}

func (s *server) handleGroup(w http.ResponseWriter, r *http.Request) {
	g := r.PathValue("group")
	if !s.allowedGroup(g) {
		http.NotFound(w, r)
		return
	}
	devs, err := config.LoadRouterDB(filepath.Join(s.cfg.BaseDir, g, "router.db"))
	if err != nil {
		http.Error(w, "router.db not readable", http.StatusInternalServerError)
		return
	}
	s.render(w, "group.html", map[string]any{
		"Title": "Group " + g,
		"Group": g,
		"Devs":  devs,
	})
}

func (s *server) handleConfig(w http.ResponseWriter, r *http.Request) {
	g := r.PathValue("group")
	h := r.PathValue("host")
	if !s.allowedGroup(g) || !hostPat.MatchString(h) {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(s.cfg.BaseDir, g, "configs", h)
	body, err := os.ReadFile(p)
	if err != nil {
		http.Error(w, "config file not found", http.StatusNotFound)
		return
	}
	s.render(w, "config.html", map[string]any{
		"Title":   fmt.Sprintf("%s / %s — config", g, h),
		"Group":   g,
		"Host":    h,
		"Path":    p,
		"Content": string(body),
	})
}

func (s *server) handleDiff(w http.ResponseWriter, r *http.Request) {
	g := r.PathValue("group")
	h := r.PathValue("host")
	if !s.allowedGroup(g) || !hostPat.MatchString(h) {
		http.NotFound(w, r)
		return
	}
	repo := filepath.Join(s.cfg.BaseDir, g)
	rel := filepath.ToSlash(filepath.Join("configs", h))
	patch, err := git.LastCommitPatch(repo, rel)
	if err != nil || len(patch) == 0 {
		patch = []byte("(no committed history for this path yet)\n")
	}
	s.render(w, "diff.html", map[string]any{
		"Title": fmt.Sprintf("%s / %s — last diff", g, h),
		"Group": g,
		"Host":  h,
		"Patch": string(patch),
	})
}

func (s *server) render(w http.ResponseWriter, name string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}
