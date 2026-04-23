package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gorancid/pkg/collect"
	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/git"
	"gorancid/pkg/parse"
)

type apiServer struct {
	cfg        config.Config
	sysconfdir string
	cloginrc   string
	timeout    time.Duration
}

func (a *apiServer) allowedGroup(name string) bool {
	return allowedGroup(a.cfg.Groups, name)
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type deviceStatus struct {
	Hostname   string `json:"hostname"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	LastCommit string `json:"last_commit"`
}

func (a *apiServer) handleGroupStatus(w http.ResponseWriter, r *http.Request) {
	g := r.PathValue("group")
	if !a.allowedGroup(g) {
		writeJSON(w, http.StatusNotFound, apiError{"unknown group"})
		return
	}
	devices, err := config.LoadRouterDB(filepath.Join(a.cfg.BaseDir, g, "router.db"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{err.Error()})
		return
	}
	repoDir := filepath.Join(a.cfg.BaseDir, g)
	result := make([]deviceStatus, 0, len(devices))
	for _, d := range devices {
		ts, _ := git.LastCommitTime(repoDir, filepath.ToSlash(filepath.Join("configs", d.Hostname)))
		var lastCommit string
		if !ts.IsZero() {
			lastCommit = ts.UTC().Format(time.RFC3339)
		}
		result = append(result, deviceStatus{
			Hostname:   d.Hostname,
			Type:       d.Type,
			Status:     d.Status,
			LastCommit: lastCommit,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Hostname < result[j].Hostname })
	writeJSON(w, http.StatusOK, result)
}

func (a *apiServer) handleDeviceConfig(w http.ResponseWriter, r *http.Request) {
	g := r.PathValue("group")
	h := r.PathValue("host")
	if !a.allowedGroup(g) {
		writeJSON(w, http.StatusNotFound, apiError{"unknown group"})
		return
	}
	if !hostPat.MatchString(h) {
		writeJSON(w, http.StatusNotFound, apiError{"not found"})
		return
	}
	body, err := os.ReadFile(filepath.Join(a.cfg.BaseDir, g, "configs", h))
	if err != nil {
		writeJSON(w, http.StatusNotFound, apiError{"config not found"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (a *apiServer) handleDeviceDiff(w http.ResponseWriter, r *http.Request) {
	g := r.PathValue("group")
	h := r.PathValue("host")
	if !a.allowedGroup(g) {
		writeJSON(w, http.StatusNotFound, apiError{"unknown group"})
		return
	}
	if !hostPat.MatchString(h) {
		writeJSON(w, http.StatusNotFound, apiError{"not found"})
		return
	}
	repo := filepath.Join(a.cfg.BaseDir, g)
	rel := filepath.ToSlash(filepath.Join("configs", h))
	patch, err := git.LastCommitPatch(repo, rel)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(patch)
}

func (a *apiServer) handleCollect(w http.ResponseWriter, r *http.Request) {
	g := r.PathValue("group")
	if !a.allowedGroup(g) {
		writeJSON(w, http.StatusNotFound, apiError{"unknown group"})
		return
	}
	hostname := r.URL.Query().Get("device")
	if hostname == "" {
		writeJSON(w, http.StatusBadRequest, apiError{"missing required query param: device"})
		return
	}
	if !hostPat.MatchString(hostname) {
		writeJSON(w, http.StatusBadRequest, apiError{"invalid device hostname"})
		return
	}

	devices, err := config.LoadRouterDB(filepath.Join(a.cfg.BaseDir, g, "router.db"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{err.Error()})
		return
	}
	var found *config.Device
	for i := range devices {
		if devices[i].Hostname == hostname {
			found = &devices[i]
			break
		}
	}
	if found == nil {
		writeJSON(w, http.StatusNotFound, apiError{"device not found in router.db"})
		return
	}

	sysconfdir := a.sysconfdir
	if sysconfdir == "" {
		sysconfdir = "/usr/local/rancid/etc"
	}
	typeSpecs, err := devicetype.Load(
		filepath.Join(sysconfdir, "rancid.types.base"),
		filepath.Join(sysconfdir, "rancid.types.conf"),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{"load device types: " + err.Error()})
		return
	}
	devicetype.RegisterMissingParsers(typeSpecs)

	spec, ok := devicetype.Lookup(typeSpecs, found.Type)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, apiError{"unknown device type: " + found.Type})
		return
	}

	var creds config.Credentials
	if cs, err := config.LoadCloginrc(a.cloginrc); err != nil {
		if a.cloginrc != "" {
			log.Printf("api: cloginrc load %s: %v", a.cloginrc, err)
		}
	} else {
		creds = cs.Lookup(found.Hostname)
	}

	timeout := a.timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	gc := &collect.GoCollector{
		Device:  *found,
		Spec:    spec,
		Creds:   creds,
		OutDir:  filepath.Join(a.cfg.BaseDir, g, "configs"),
		Timeout: timeout,
		FilterOpts: parse.FilterOpts{
			FilterPwds: int(a.cfg.FilterPwds),
			FilterOsc:  int(a.cfg.FilterOsc),
			NoCommStr:  a.cfg.NoCommStr,
		},
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout+5*time.Second)
	defer cancel()

	result, err := gc.Run(ctx)
	if err != nil || result.Error != nil {
		msg := ""
		if err != nil {
			msg = err.Error()
		} else {
			msg = result.Error.Error()
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"hostname": hostname,
			"status":   "failed",
			"error":    msg,
		})
		return
	}

	repoDir := filepath.Join(a.cfg.BaseDir, g)
	configRel := filepath.ToSlash(filepath.Join("configs", hostname))
	_ = git.Add(repoDir, []string{configRel})
	diff, _ := git.Diff(repoDir, configRel)
	if len(diff) > 0 {
		_ = git.Commit(repoDir, "api collect "+hostname)
	}

	resp := map[string]string{"hostname": hostname, "status": "ok"}
	if len(diff) > 0 {
		resp["diff"] = string(diff)
	}
	writeJSON(w, http.StatusOK, resp)
}
