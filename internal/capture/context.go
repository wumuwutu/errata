package capture

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Context is the "scene" of an error: what was run, where, and on what.
type Context struct {
	Command   string // full command line as invoked
	Dir       string // working directory
	GitCommit string // short HEAD hash, empty outside a git repo
	Runtime   string // e.g. "Python 3.12.4" or "v22.5.1" for node, best-effort
	OS        string // GOOS/GOARCH
}

// Scene collects the context for a command line, best-effort: any signal
// that cannot be obtained is simply left empty.
func Scene(args []string) Context {
	ctx := Context{
		Command: strings.Join(args, " "),
		OS:      runtime.GOOS + "/" + runtime.GOARCH,
	}
	if dir, err := os.Getwd(); err == nil {
		ctx.Dir = dir
		ctx.GitCommit = gitHead(dir)
	}
	ctx.Runtime = runtimeVersion(filepath.Base(args[0]))
	return ctx
}

// gitHead returns the short HEAD hash if dir is inside a git repository.
func gitHead(dir string) string {
	check := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	if out, err := check.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return ""
	}
	head := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD")
	out, err := head.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runtimeVersion probes the interpreter version for the runtimes dejavu
// fingerprints (python/node). Anything else yields "".
func runtimeVersion(base string) string {
	var probe string
	switch base {
	case "python", "python3":
		probe = base
	case "node":
		probe = "node"
	default:
		return ""
	}
	out, err := exec.Command(probe, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
