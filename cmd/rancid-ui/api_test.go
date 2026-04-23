package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gorancid/pkg/config"
	"gorancid/pkg/git"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected value, got %s", result["key"])
	}
}

func TestAPIServerAllowedGroup(t *testing.T) {
	a := &apiServer{cfg: config.Config{Groups: []string{"observium", "lab"}}}
	if !a.allowedGroup("observium") {
		t.Error("expected observium to be allowed")
	}
	if a.allowedGroup("unknown") {
		t.Error("expected unknown to be rejected")
	}
}

func TestHandleGroupStatusUnknownGroup(t *testing.T) {
	a := &apiServer{cfg: config.Config{Groups: []string{"observium"}}}
	req := httptest.NewRequest("GET", "/api/v1/groups/unknown/status", nil)
	req.SetPathValue("group", "unknown")
	w := httptest.NewRecorder()
	a.handleGroupStatus(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleGroupStatus(t *testing.T) {
	dir := t.TempDir()
	groupDir := filepath.Join(dir, "observium")
	configsDir := filepath.Join(groupDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "router.db"),
		[]byte("ac2401;cisco;up\nag241;junos;down\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := git.Init(groupDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configsDir, "ac2401"), []byte("hostname ac2401\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := git.Add(groupDir, []string{"configs/ac2401"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := git.Commit(groupDir, "collect ac2401"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	a := &apiServer{cfg: config.Config{BaseDir: dir, Groups: []string{"observium"}}}
	req := httptest.NewRequest("GET", "/api/v1/groups/observium/status", nil)
	req.SetPathValue("group", "observium")
	w := httptest.NewRecorder()
	a.handleGroupStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(result))
	}
	if result[0]["hostname"] != "ac2401" {
		t.Errorf("expected ac2401 first, got %s", result[0]["hostname"])
	}
	if result[0]["last_commit"] == "" {
		t.Error("expected non-empty last_commit for ac2401")
	}
	if result[1]["hostname"] != "ag241" {
		t.Errorf("expected ag241 second, got %s", result[1]["hostname"])
	}
	if result[1]["last_commit"] != "" {
		t.Errorf("expected empty last_commit for ag241, got %s", result[1]["last_commit"])
	}
}
