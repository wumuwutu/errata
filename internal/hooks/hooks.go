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

//go:embed scripts/errata.zsh
var zshScript string

//go:embed scripts/errata.bash
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

// rcBlock is the exact block WriteRC appends, so uninstall can remove it
// precisely (uninstall red line, dev-guide §9).
func rcBlock(shell string) string {
	return fmt.Sprintf("\n# errata shell hook — https://github.com/wumuwutu/errata\n%s\n", EvalLine(shell))
}

// legacyRCBlock is the block appended before the product rename (its
// comment word and repo URL still say "dejavu"); RemoveRC must recognize
// it too, so old installations uninstall cleanly.
func legacyRCBlock(shell string) string {
	return fmt.Sprintf("\n# dejavu shell hook — https://github.com/wumuwutu/dejavu\n%s\n", EvalLine(shell))
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
	if _, err := f.WriteString(rcBlock(shell)); err != nil {
		return "", false, err
	}
	return rcPath, false, nil
}

// RemoveRC removes the hook block from the shell's rc file — the exact
// block WriteRC appended, or a bare eval line the user added by hand.
// removed reports whether anything changed.
func RemoveRC(shell string) (rcPath string, removed bool, err error) {
	rcPath, err = RCFile(shell)
	if err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return rcPath, false, nil
		}
		return "", false, err
	}
	content := string(data)
	var out string
	switch {
	case strings.Contains(content, rcBlock(shell)):
		out = strings.Replace(content, rcBlock(shell), "", 1)
	case strings.Contains(content, legacyRCBlock(shell)):
		out = strings.Replace(content, legacyRCBlock(shell), "", 1)
	case strings.Contains(content, EvalLine(shell)+"\n"):
		out = strings.Replace(content, EvalLine(shell)+"\n", "", 1)
	case strings.Contains(content, EvalLine(shell)):
		out = strings.Replace(content, EvalLine(shell), "", 1)
	default:
		return rcPath, false, nil
	}
	if err := os.WriteFile(rcPath, []byte(out), 0o644); err != nil {
		return "", false, err
	}
	return rcPath, true, nil
}
