package main

import (
	"encoding/json"
	"net/http"
	"time"

	"gorancid/pkg/config"
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
