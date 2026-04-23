package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorancid/pkg/config"
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
