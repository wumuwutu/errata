package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/wumuwutu/errata/internal/hooks"
	"github.com/wumuwutu/errata/internal/store"
	"github.com/wumuwutu/errata/internal/termx"
)

// maxDrafts caps the numbered candidate list (dev-guide §7.3: precision
// first — three good guesses beat ten noisy ones).
const maxDrafts = 3

// solutionDrafts implements dev-guide §7.3 "command-history inference":
// the commands run between the error and now are the strongest hint for
// its solution (pip install numpy). It reads this shell session's command
// log (the hook appends one `epoch<TAB>command` line per command to
// sess-<pid>.cmds) and returns up to maxDrafts candidates. Every
// failure — no hooked session (err run, pipes), no log file, nothing
// relevant — silently yields no draft.
func solutionDrafts(e *store.Error) []string {
	path, ok := hooks.CommandsLogPath(os.Getenv(hooks.SessionEnvVar))
	if !ok {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // no log for this session: err run, another terminal
	}
	return pickDrafts(string(data), e)
}

// pickDrafts is the filter funnel, precision first:
//  1. keep only commands logged at or after the error's last_seen
//     (same-second commands survive; the failing command itself is
//     dropped by text, not by the clock);
//  2. drop view/navigation commands, err itself and the failed command;
//  3. rank environment-changing commands (pip/npm install, export,
//     systemctl, source) first, then commands sharing the failed
//     command's program, then the rest — chronological within a tier;
//  4. keep at most maxDrafts, exact duplicates removed.
func pickDrafts(log string, e *store.Error) []string {
	since := e.LastSeen.Unix()
	seen := map[string]bool{e.Command: true} // the failing command itself
	var tiers [3][]string
	for _, line := range strings.Split(log, "\n") {
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue // malformed line (e.g. a multi-line command): skip
		}
		epoch, err := strconv.ParseInt(line[:tab], 10, 64)
		if err != nil || epoch < since {
			continue
		}
		cmd := strings.TrimSpace(line[tab+1:])
		if cmd == "" || seen[cmd] || isNoiseCommand(cmd) {
			continue
		}
		seen[cmd] = true
		tier := draftTier(cmd, e.Command)
		tiers[tier] = append(tiers[tier], cmd)
	}
	var out []string
	for _, tier := range tiers {
		out = append(out, tier...)
	}
	if len(out) > maxDrafts {
		out = out[:maxDrafts]
	}
	return out
}

// printDrafts shows the numbered candidates in faint gray; entering the
// number at the solution prompt adopts that command as the solution.
func printDrafts(w io.Writer, drafts []string) {
	if len(drafts) == 0 {
		return
	}
	fmt.Fprintln(w, termx.Faint("  since the error you ran:"))
	for i, d := range drafts {
		fmt.Fprintln(w, termx.Faint(fmt.Sprintf("    %d. %s", i+1, termx.Truncate(d, 72))))
	}
}

// viewOnlyCommands are pure inspection/navigation: they never fix
// anything, so they only add noise to the draft list.
var viewOnlyCommands = map[string]bool{
	"ls": true, "ll": true, "cd": true, "pwd": true,
	"cat": true, "less": true, "more": true, "head": true, "tail": true,
	"echo": true, "printf": true, "which": true, "type": true,
	"history": true, "man": true, "clear": true,
	"true": true, "false": true, "exit": true, "logout": true,
	"env": true, "printenv": true,
}

// isNoiseCommand reports whether a command can never be a solution:
// view-only commands, `export` without assignments (export -p just
// prints), and err itself.
func isNoiseCommand(cmd string) bool {
	prog := programOf(cmd)
	if viewOnlyCommands[prog] {
		return true
	}
	if prog == "export" && !strings.Contains(cmd, "=") {
		return true
	}
	return prog == "err" || prog == "errata"
}

// draftTier ranks a candidate: 0 for environment-changing commands
// (package installs, export FOO=1, systemctl, source), 1 for commands
// sharing the failed command's program, 2 for everything else.
func draftTier(cmd, failedCmd string) int {
	if isEnvChange(cmd) {
		return 0
	}
	if programOf(cmd) == programOf(failedCmd) {
		return 1
	}
	return 2
}

// packageManagers change the machine/environment with install-ish
// subcommands (pip install, npm add, apt upgrade).
var packageManagers = map[string]bool{
	"pip": true, "npm": true, "yarn": true, "pnpm": true, "bun": true,
	"apt": true, "apt-get": true, "dnf": true, "yum": true, "pacman": true,
	"brew": true, "cargo": true, "conda": true, "mamba": true,
	"gem": true, "pipx": true, "uv": true, "go": true,
}

// envSubcommands are the package-manager verbs that change the
// environment the failed command runs in.
var envSubcommands = map[string]bool{
	"install": true, "add": true, "remove": true, "uninstall": true,
	"upgrade": true, "update": true,
}

// isEnvChange reports whether a command plausibly changed the environment
// the failed command runs in — the highest-value draft tier.
func isEnvChange(cmd string) bool {
	fields := strings.Fields(cmd)
	i := 0
	for i < len(fields) && isAssignment(fields[i]) {
		i++
	}
	if i >= len(fields) {
		return false
	}
	switch fields[i] {
	case "source", ".", "systemctl":
		return true
	}
	prog := programOf(cmd)
	if prog == "export" {
		return strings.Contains(cmd, "=") // export FOO=1 changes env
	}
	if !packageManagers[prog] {
		return false
	}
	for _, f := range fields[i+1:] {
		if envSubcommands[f] {
			return true
		}
	}
	return false
}
