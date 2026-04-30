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
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	file := filepath.Join(dir, "router.cfg")
	if err := os.WriteFile(file, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := git.Add(dir, []string{"router.cfg"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := git.Commit(dir, "initial"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

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

func TestSetRemote_Add(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := git.SetRemote(dir, "origin", "git@git.example.com:org/repo.git"); err != nil {
		t.Fatalf("SetRemote (add): %v", err)
	}

	url, err := git.RemoteURL(dir, "origin")
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if url != "git@git.example.com:org/repo.git" {
		t.Errorf("after add: got %q, want %q", url, "git@git.example.com:org/repo.git")
	}
}

func TestSetRemote_Update(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	_ = git.AddRemote(dir, "origin", "git@git.example.com:org/old.git")

	if err := git.SetRemote(dir, "origin", "git@git.example.com:org/new.git"); err != nil {
		t.Fatalf("SetRemote (update): %v", err)
	}

	url, err := git.RemoteURL(dir, "origin")
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if url != "git@git.example.com:org/new.git" {
		t.Errorf("after update: got %q, want %q", url, "git@git.example.com:org/new.git")
	}
}

func TestAddRemote(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := git.AddRemote(dir, "origin", "git@git.example.com:org/repo.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	// Verify the remote was actually registered.
	url, err := git.RemoteURL(dir, "origin")
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if url != "git@git.example.com:org/repo.git" {
		t.Errorf("expected remote URL %q, got %q", "git@git.example.com:org/repo.git", url)
	}
}

func TestPush(t *testing.T) {
	// Use a local bare repo as a stand-in for the remote.
	remote := t.TempDir()
	if err := git.InitBare(remote); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	local := t.TempDir()
	if err := git.Init(local); err != nil {
		t.Fatalf("Init: %v", err)
	}

	file := filepath.Join(local, "router.cfg")
	if err := os.WriteFile(file, []byte("hostname switch1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := git.Add(local, []string{"router.cfg"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := git.Commit(local, "initial config"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := git.AddRemote(local, "origin", remote); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	if err := git.Push(local, "origin", "main"); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify the remote received the commit.
	ts, err := git.LastCommitTime(remote, "")
	if err != nil {
		t.Fatalf("LastCommitTime on remote: %v", err)
	}
	if ts.IsZero() {
		t.Error("expected remote to have a commit after push")
	}
}
