package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestProgramOf(t *testing.T) {
	cases := map[string]string{
		"python3 train.py":        "python",
		"python3.11 train.py":     "python",
		"python train.py":         "python",
		"/usr/bin/python3 x.py":   "python",
		"node app.js":             "node",
		"bash -c 'echo hi'":       "bash",
		"git status":              "git",
		"":                        "",
		"  ":                      "",
		"VAR=1 python3 script.py": "python", // env prefix skipped
	}
	for in, want := range cases {
		if got := programOf(in); got != want {
			t.Errorf("programOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSolvedHintProgramGate: the nudge fires only when the successful
// command's program matches the pending error's program.
func TestSolvedHintProgramGate(t *testing.T) {
	setupTestStore(t) // seeds one pending python error in /tmp/proj

	dir := "/tmp/proj"
	// The seeded error was recorded with command "python app.py" in dir
	// "/tmp/proj" (see setupTestStore).

	var buf bytes.Buffer

	// Unrelated program: silence.
	solvedHint(dir, "ls -la", &buf)
	if buf.Len() != 0 {
		t.Fatalf("unrelated program nudged: %q", buf.String())
	}
	solvedHint(dir, "node app.js", &buf)
	if buf.Len() != 0 {
		t.Fatalf("node success must not nudge a python error: %q", buf.String())
	}

	// Same program (python vs python3 alias): nudge.
	solvedHint(dir, "python3 app.py", &buf)
	if !strings.Contains(buf.String(), "looks fixed") {
		t.Fatalf("same-program success should nudge: %q", buf.String())
	}

	// 24h remind-once: immediate repeat stays silent.
	buf.Reset()
	solvedHint(dir, "python app.py", &buf)
	if buf.Len() != 0 {
		t.Fatalf("second nudge within 24h: %q", buf.String())
	}
}
