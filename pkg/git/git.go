package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Init initializes a new git repository in dir.
func Init(dir string) error {
	if err := run(dir, "git", "init"); err != nil {
		return err
	}
	// Set required git identity for commits to work in isolated environments.
	_ = run(dir, "git", "config", "user.email", "rancid@localhost")
	_ = run(dir, "git", "config", "user.name", "rancid")
	return nil
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
// or zero time if no such commit exists.
func LastCommitTime(dir, path string) (time.Time, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%cI", "--", path)
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

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return nil
}
