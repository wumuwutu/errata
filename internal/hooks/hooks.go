// Package hooks provides the shell hook scripts for `err init` and the
// rc-file writer for --write. The scripts live in scripts/ and are
// embedded verbatim so tests exercise exactly what users get.
package hooks

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed scripts/dejavu.zsh
var zshScript string

//go:embed scripts/dejavu.bash
var bashScript string

// Supported shells for hooks, in display order.
var Supported = []string{"zsh", "bash"}

// Script returns the hook script for shell. ok is false for unsupported
// shells (fish & friends: fail gracefully, never break the user's rc).
func Script(shell string) (script string, ok bool) {
	switch strings.ToLower(shell) {
	case "zsh":
		return zshScript, true
	case "bash":
		return bashScript, true
	default:
		return "", false
	}
}

// EvalLine is the rc line users add: eval "$(err init <shell>)".
func EvalLine(shell string) string {
	return fmt.Sprintf(`eval "$(err init %s)"`, shell)
}

// RCFile returns the rc file err init --write appends to.
func RCFile(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch strings.ToLower(shell) {
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

// WriteRC appends the eval line to the shell's rc file. It reports the
// file touched and whether the line was already present (then nothing is
// written). The rc file is created if missing.
func WriteRC(shell string) (rcPath string, already bool, err error) {
	rcPath, err = RCFile(shell)
	if err != nil {
		return "", false, err
	}
	line := EvalLine(shell)

	data, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	if strings.Contains(string(data), line) {
		return rcPath, true, nil
	}

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	block := fmt.Sprintf("\n# dejavu shell hook — https://github.com/wumuwutu/dejavu\n%s\n", line)
	if _, err := f.WriteString(block); err != nil {
		return "", false, err
	}
	return rcPath, false, nil
}
