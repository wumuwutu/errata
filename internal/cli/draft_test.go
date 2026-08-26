package cli

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wumuwutu/errata/internal/store"
)

// draftLog renders `epoch<TAB>command` lines starting at base, one second
// apart, for pickDrafts tests.
func draftLog(base int64, cmds ...string) string {
	var b strings.Builder
	for i, c := range cmds {
		b.WriteString(strconv.FormatInt(base+int64(i), 10))
		b.WriteByte('\t')
		b.WriteString(c)
		b.WriteByte('\n')
	}
	return b.String()
}

func draftError(lastSeen time.Time) *store.Error {
	return &store.Error{Command: "python3 train.py", LastSeen: lastSeen}
}

func TestPickDraftsFunnel(t *testing.T) {
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	e := draftError(base.Add(2 * time.Second)) // last_seen at base+2s
	log := draftLog(base.Unix(),
		"git status",               // base+0: before the error — dropped by the clock
		"ls -la",                   // base+1: before the error — dropped
		"python3 train.py",         // base+2: the failing command itself — dropped by text
		"ls",                       // noise
		"export -p",                // noise (prints, changes nothing)
		"err pending",              // err itself
		"git diff",                 // tier 2
		"python3 -c 'import numpy'", // tier 1 (same program)
		"pip install numpy",        // tier 0 (environment change)
	)
	got := pickDrafts(log, e)
	want := []string{"pip install numpy", "python3 -c 'import numpy'", "git diff"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pickDrafts = %v, want %v", got, want)
	}
}

func TestPickDraftsCapAtThree(t *testing.T) {
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	e := draftError(base)
	got := pickDrafts(draftLog(base.Unix(),
		"pip install a", "npm install b", "cargo install c",
		"brew install d", "apt install e",
	), e)
	want := []string{"pip install a", "npm install b", "cargo install c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cap: got %v, want %v", got, want)
	}
}

func TestPickDraftsNoiseOnlyYieldsNothing(t *testing.T) {
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	e := draftError(base)
	if got := pickDrafts(draftLog(base.Unix(),
		"ls", "cd ..", "pwd", "cat log.txt", "echo hi", "history",
		"true", "err pending", "python3 train.py",
	), e); len(got) != 0 {
		t.Fatalf("noise-only log must yield no draft, got %v", got)
	}
}

func TestPickDraftsMalformedAndDedupe(t *testing.T) {
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	e := draftError(base)
	log := "no-tab line\n" +
		"notanumber\tpip install skipped\n" +
		draftLog(base.Unix(), "pip install numpy", "pip install numpy")
	got := pickDrafts(log, e)
	want := []string{"pip install numpy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("malformed/dedupe: got %v, want %v", got, want)
	}
}

func TestIsNoiseCommand(t *testing.T) {
	for _, cmd := range []string{
		"ls", "ls -la", "cd ..", "pwd", "cat f", "echo hi", "which python3",
		"history", "man ls", "clear", "true", "false", "exit",
		"export -p", "export", "err pending", "err fix 3", "errata show 1",
	} {
		if !isNoiseCommand(cmd) {
			t.Errorf("%q should be noise", cmd)
		}
	}
	for _, cmd := range []string{
		"export FOO=1", "pip install numpy", "python3 train.py", "git status",
	} {
		if isNoiseCommand(cmd) {
			t.Errorf("%q must not be noise", cmd)
		}
	}
}

func TestDraftTier(t *testing.T) {
	failed := "python3 train.py"
	for cmd, want := range map[string]int{
		"pip install numpy":      0,
		"pip3 install numpy":     0, // version suffix normalized
		"npm run build":          2, // package manager, but no install-ish verb
		"apt-get upgrade":        0,
		"brew install wget":      0,
		"cargo add serde":        0,
		"go install ./cmd/x":     0,
		"export LD_PRELOAD=/x":   0,
		"systemctl restart sshd": 0,
		"source .venv/bin/activate": 0,
		". venv/bin/activate":       0,
		"python3 -m pytest":         1, // same program as the failed command
		"python train.py --debug":   1,
		"git checkout main":         2,
		"vim train.py":              2,
	} {
		if got := draftTier(cmd, failed); got != want {
			t.Errorf("draftTier(%q) = %d, want %d", cmd, got, want)
		}
	}
}

func TestPickDraftSolution(t *testing.T) {
	drafts := []string{"pip install numpy", "python3 -c 'import numpy'"}
	if got := pickDraftSolution("1", drafts); got != drafts[0] {
		t.Fatalf("pick 1: %q", got)
	}
	if got := pickDraftSolution("2", drafts); got != drafts[1] {
		t.Fatalf("pick 2: %q", got)
	}
	// Out of range or no drafts at all: the text is the solution.
	if got := pickDraftSolution("3", drafts); got != "3" {
		t.Fatalf("out of range must stay literal: %q", got)
	}
	if got := pickDraftSolution("1", nil); got != "1" {
		t.Fatalf("no drafts must stay literal: %q", got)
	}
	if got := pickDraftSolution("pin torch==2.1", drafts); got != "pin torch==2.1" {
		t.Fatalf("free text: %q", got)
	}
}
