package git_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestPushContext_AlreadyCanceled(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := git.PushContext(ctx, dir, "origin", "main")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestPushContext_DeadlineExceeded(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				time.Sleep(5 * time.Minute)
			}(c)
		}
	}()

	host := ln.Addr().String()
	remoteURL := fmt.Sprintf("http://%s/dummy.git", host)

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
	if err := git.Commit(local, "initial"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := git.SetRemote(local, "origin", remoteURL); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	pushErr := git.PushContext(ctx, local, "origin", "main")
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected expired push context, ctx.Err()=%v", ctx.Err())
	}
	if pushErr == nil {
		t.Fatal("expected non-nil error from timed-out push")
	}

	// GIT_PUSH_TIMEOUT must tear down the whole process group. Killing only the
	// parent `git` leaves orphaned git-remote-http / send-pack helpers (seen on
	// Observium as hundreds of PPID-1 processes lasting weeks).
	deadline := time.Now().Add(3 * time.Second)
	for {
		left := countProcsMatching(remoteURL)
		if left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed-out push left %d orphan git helper process(es) matching %q", left, remoteURL)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// countProcsMatching returns how many processes have cmdlines containing frag.
func countProcsMatching(frag string) int {
	out, err := exec.Command("ps", "-ax", "-o", "command=").Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, frag) && (strings.Contains(line, "git-remote-http") ||
			strings.Contains(line, "git send-pack") ||
			strings.Contains(line, "git remote-http") ||
			strings.Contains(line, "git push")) {
			n++
		}
	}
	return n
}

func TestPathChanged(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	rdb := filepath.Join(dir, "router.db")
	if err := os.WriteFile(rdb, []byte("host;ios;up\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !git.PathChanged(dir, "router.db") {
		t.Fatal("expected untracked router.db to be reported as changed")
	}
	if err := git.Add(dir, []string{"router.db"}); err != nil {
		t.Fatal(err)
	}
	if err := git.Commit(dir, "add router.db"); err != nil {
		t.Fatal(err)
	}
	if git.PathChanged(dir, "router.db") {
		t.Fatal("expected clean router.db after commit")
	}
	if err := os.WriteFile(rdb, []byte("host;ios;up\nhost2;junos;up\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !git.PathChanged(dir, "router.db") {
		t.Fatal("expected modified router.db to be reported as changed")
	}
}
