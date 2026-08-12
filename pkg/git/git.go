package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Init initializes a new git repository in dir with branch name "main".
func Init(dir string) error {
	if err := run(dir, "git", "init", "-b", "main"); err != nil {
		return err
	}
	// Set required git identity for commits to work in isolated environments.
	_ = run(dir, "git", "config", "user.email", "rancid@localhost")
	_ = run(dir, "git", "config", "user.name", "rancid")
	return nil
}

// InitBare initializes a bare git repository in dir (suitable as a remote).
func InitBare(dir string) error {
	return run(dir, "git", "init", "--bare", "-b", "main")
}

// AddRemote adds a named remote to the repository in dir.
func AddRemote(dir, name, url string) error {
	return run(dir, "git", "remote", "add", name, url)
}

// RemoteURL returns the fetch URL of the named remote.
func RemoteURL(dir, name string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", name)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SetRemote adds the named remote if it does not exist, or updates its URL if it does.
func SetRemote(dir, name, url string) error {
	existing, err := RemoteURL(dir, name)
	if err == nil {
		if existing == url {
			return nil
		}
		return run(dir, "git", "remote", "set-url", name, url)
	}
	return run(dir, "git", "remote", "add", name, url)
}

// Push pushes branch to remote and waits until git exits (no deadline).
func Push(dir, remote, branch string) error {
	return PushContext(context.Background(), dir, remote, branch)
}

// PushContext runs git push under ctx. When ctx is cancelled or its deadline passes,
// the whole process group is SIGKILL'd (Unix) so wedged HTTPS helpers
// (git-remote-http / send-pack) cannot outlive control-rancid as PID-1 orphans.
func PushContext(ctx context.Context, dir, remote, branch string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("git push %s %s: %w", remote, branch, err)
	}

	cmd := exec.Command("git", "push", remote, branch)
	cmd.Dir = dir
	setProcessGroup(cmd)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git push %s %s: %w", remote, branch, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("git push %s %s: %w\n%s", remote, branch, err, buf.Bytes())
		}
		return nil
	case <-ctx.Done():
		killProcessGroup(cmd.Process.Pid)
		select {
		case <-done:
		case <-time.After(15 * time.Second):
		}
		return fmt.Errorf("git push %s %s: %w\n%s", remote, branch, ctx.Err(), buf.Bytes())
	}
}

// Add stages files for commit.
func Add(dir string, files []string) error {
	args := append([]string{"add", "--"}, files...)
	return run(dir, "git", args...)
}

// Commit commits all staged changes with message.
func Commit(dir, message string) error {
	return run(dir, "git", "commit", "-m", message)
}

// Diff returns the staged diff for file. Returns empty bytes if no changes.
func Diff(dir, file string) ([]byte, error) {
	cmd := exec.Command("git", "diff", "--cached", "--", file)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out, nil
		}
		return nil, fmt.Errorf("git diff: %w", err)
	}
	return out, nil
}

// LastCommitPatch returns the unified diff introduced by the latest commit that touched path.
func LastCommitPatch(dir, path string) ([]byte, error) {
	cmd := exec.Command("git", "log", "-1", "-p", "--follow", "--", path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, nil
		}
		return nil, fmt.Errorf("git log -p: %w", err)
	}
	return out, nil
}

// LastCommitTime returns the timestamp of the most recent commit that touched path,
// or zero time if no such commit exists. Pass an empty path to get the most recent
// commit in the repository regardless of which file it touched.
func LastCommitTime(dir, path string) (time.Time, error) {
	args := []string{"log", "-1", "--format=%cI"}
	if path != "" {
		args = append(args, "--", path)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("git log: %w", err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// PathChanged reports whether relpath has unstaged or staged changes in dir.
func PathChanged(dir, relpath string) bool {
	cmd := exec.Command("git", "status", "--porcelain", "--", relpath)
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func run(dir, name string, args ...string) error {
	return runGit(context.Background(), dir, name, args...)
}

func runGit(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return nil
}
