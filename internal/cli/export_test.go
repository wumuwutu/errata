package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wumuwutu/errata/internal/store"
)

func TestRenderExportGroupsAndOrders(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return time.Date(2026, 8, d, 10, 0, 0, 0, time.UTC) }
	items := []store.Error{
		{ID: 2, Signature: "KeyError: 'x'", Language: "python", ProjectDir: "/proj",
			Count: 3, FirstSeen: day(20), LastSeen: day(22)},
		{ID: 1, Signature: "TypeError: boom", Language: "python", ProjectDir: "/proj",
			Count: 5, FirstSeen: day(10), LastSeen: day(21), Solution: "pin the version"},
		{ID: 3, Signature: "panic: runtime error", Language: "go", ProjectDir: "/other",
			Count: 1, FirstSeen: day(15), LastSeen: day(15)},
	}
	out := renderExport(items, now)

	for _, want := range []string{
		"# errata error library",
		"exported: 2026-08-25 09:00:00",
		"errors: 3 total, 1 with a solution",
		"## /proj", "## /other",
		"solution: pin the version",
		"solution: *(pending)*",
		"seen: 5 times (first 2026-08-10 10:00:00, last 2026-08-21 10:00:00)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q:\n%s", want, out)
		}
	}
	// Within a project, oldest first.
	if strings.Index(out, "#1") > strings.Index(out, "#2") {
		t.Errorf("entries not in chronological order:\n%s", out)
	}
	// Plain text: no ANSI escapes, ever.
	if strings.Contains(out, "\x1b[") {
		t.Error("export must not contain ANSI escapes")
	}
}

func TestResolveExportPath(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()

	p, err := resolveExportPath("", now)
	if err != nil || p != "errata-export-20260825.md" {
		t.Fatalf("default: %q %v", p, err)
	}

	p, err = resolveExportPath(dir, now)
	if err != nil || p != filepath.Join(dir, "errata-export-20260825.md") {
		t.Fatalf("existing dir: %q %v", p, err)
	}

	p, err = resolveExportPath(filepath.Join(dir, "out.md"), now)
	if err != nil || p != filepath.Join(dir, "out.md") {
		t.Fatalf("new file in existing dir: %q %v", p, err)
	}

	if _, err := resolveExportPath(filepath.Join(dir, "nope", "out.md"), now); err == nil {
		t.Fatal("missing parent directory must be an error")
	}
	if _, err := resolveExportPath(filepath.Join(dir, "nope")+"/", now); err == nil {
		t.Fatal("missing directory (trailing slash) must be an error")
	}
}

func TestExportCommandWritesFile(t *testing.T) {
	setupTestStore(t) // seeds one pending python error in /tmp/proj
	dir := t.TempDir()
	out := filepath.Join(dir, "lib.md")

	old := exportOutput
	exportOutput = out
	defer func() { exportOutput = old }()

	var buf bytes.Buffer
	exportCmd.SetOut(&buf)
	defer exportCmd.SetOut(nil)

	if err := exportCmd.RunE(exportCmd, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "TypeError: cannot add <VAL>") {
		t.Fatalf("export missing seeded error:\n%s", data)
	}
	if !strings.Contains(buf.String(), "exported 1 errors to ") {
		t.Fatalf("unexpected summary line: %q", buf.String())
	}
}

func TestExportCommandBadPathFails(t *testing.T) {
	setupTestStore(t)
	old := exportOutput
	exportOutput = filepath.Join(t.TempDir(), "nope", "out.md")
	defer func() { exportOutput = old }()

	err := exportCmd.RunE(exportCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "no such directory") {
		t.Fatalf("err = %v, want a clear 'no such directory' error", err)
	}
}
