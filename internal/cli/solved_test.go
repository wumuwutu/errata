package cli

import (
	"bytes"
	"reflect"
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

func TestTargetArgs(t *testing.T) {
	cases := map[string][]string{
		"python demo7.py":                 {"demo7.py"},
		"python ./demo7.py":               {"demo7.py"},
		`python "demo7.py"`:               {"demo7.py"},
		"python3 -u demo7.py":             {"demo7.py"}, // flag ignored
		"FIXED=1 python demo7.py":         {"demo7.py"}, // env prefix skipped
		"pip install -r requirements.txt": {"install", "requirements.txt"},
		"python -m pytest":                {"pytest"}, // flag value counts
		"ls -la":                          nil,        // no targets
		"pip":                             nil,
		"":                                nil,
	}
	for in, want := range cases {
		if got := targetArgs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("targetArgs(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSameTarget(t *testing.T) {
	cases := []struct {
		failed, ok string
		want       bool
	}{
		{"python demo7.py", "python3 demo7.py", true},   // same script
		{"python demo7.py", "python3 ./demo7.py", true}, // path spelling
		{"python demo7.py", "python3 -u demo7.py", true},
		{"python demo7.py", "python3 other.py", false}, // different script
		{"python demo7.py", "python3", false},          // one side has no target
		{"python demo7.py", "vim demo7.py", false},     // different program
		{"pip", "pip", true},                           // no targets: program alone
		{"pip", "pip3", true},                          // alias, no targets
		{"pip", "npm", false},
		{"python -m pytest", "python3 -m pytest", true}, // shared flag-value target
	}
	for _, c := range cases {
		if got := sameTarget(c.failed, c.ok); got != c.want {
			t.Errorf("sameTarget(%q, %q) = %v, want %v", c.failed, c.ok, got, c.want)
		}
	}
}

// TestSolvedHintTargetGate: the nudge fires only when the successful
// command shares a target (script) with the pending error's command —
// same program alone is not enough anymore.
func TestSolvedHintTargetGate(t *testing.T) {
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

	// Same program, different script: silence (the v0.1.8 tightening).
	solvedHint(dir, "python3 other.py", &buf)
	if buf.Len() != 0 {
		t.Fatalf("same program but different script must not nudge: %q", buf.String())
	}
	// Same program, no target on the success side: silence.
	solvedHint(dir, "python3", &buf)
	if buf.Len() != 0 {
		t.Fatalf("targetless success must not nudge a targeted error: %q", buf.String())
	}

	// Same program AND same script (flags and aliases irrelevant): nudge.
	solvedHint(dir, "python3 -u app.py", &buf)
	if !strings.Contains(buf.String(), "looks fixed") {
		t.Fatalf("same-script success should nudge: %q", buf.String())
	}

	// 24h remind-once: immediate repeat stays silent.
	buf.Reset()
	solvedHint(dir, "python app.py", &buf)
	if buf.Len() != 0 {
		t.Fatalf("second nudge within 24h: %q", buf.String())
	}
}
