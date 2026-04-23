package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gorancid/pkg/config"
	"gorancid/pkg/git"
)

type apiServer struct {
	cfg        config.Config
	sysconfdir string
	cloginrc   string
	timeout    time.Duration
}

func (a *apiServer) allowedGroup(name string) bool {
	for _, g := range a.cfg.Groups {
		if g == name {
			return true
		}
	}
	return false
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
		ts, _ := git.LastCommitTime(repoDir, filepath.Join("configs", d.Hostname))
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
	if !a.allowedGroup(g) || !hostPat.MatchString(h) {
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
