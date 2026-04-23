package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"gorancid/pkg/git"
)

func TestGitWorkflow(t *testing.T) {
	dir := t.TempDir()

	// init
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// write a file and add+commit it
	file := filepath.Join(dir, "router.cfg")
	if err := os.WriteFile(file, []byte("version 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := git.Add(dir, []string{"router.cfg"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := git.Commit(dir, "initial config"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// modify and get diff
	if err := os.WriteFile(file, []byte("version 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := git.Add(dir, []string{"router.cfg"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	diff, err := git.Diff(dir, "router.cfg")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diff) == 0 {
		t.Error("expected non-empty diff after modifying file")
	}
}

func TestDiffNoChanges(t *testing.T) {
	dir := t.TempDir()
	_ = git.Init(dir)
	file := filepath.Join(dir, "router.cfg")
	_ = os.WriteFile(file, []byte("version 1\n"), 0644)
	_ = git.Add(dir, []string{"router.cfg"})
	_ = git.Commit(dir, "initial")

	diff, err := git.Diff(dir, "router.cfg")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diff) != 0 {
		t.Errorf("expected empty diff with no changes, got %q", diff)
	}
}

func TestLastCommitTime(t *testing.T) {
	dir := t.TempDir()
	_ = git.Init(dir)
	file := filepath.Join(dir, "router.cfg")
	_ = os.WriteFile(file, []byte("v1\n"), 0644)
	_ = git.Add(dir, []string{"router.cfg"})
	_ = git.Commit(dir, "initial")

	ts, err := git.LastCommitTime(dir, "router.cfg")
	if err != nil {
		t.Fatalf("LastCommitTime: %v", err)
	}
	if ts.IsZero() {
		t.Error("expected non-zero time for committed file")
	}
}

func TestLastCommitTimeNoHistory(t *testing.T) {
	dir := t.TempDir()
	_ = git.Init(dir)

	ts, err := git.LastCommitTime(dir, "nonexistent.cfg")
	if err != nil {
		t.Fatalf("LastCommitTime: %v", err)
	}
	if !ts.IsZero() {
		t.Errorf("expected zero time for nonexistent path, got %v", ts)
	}
}