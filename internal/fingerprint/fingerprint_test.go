package fingerprint

import (
	"strconv"
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mTypeError\x1b[0m: \x1b[1mboom\x1b[0m\r\n"
	got := StripANSI(in)
	if got != "TypeError: boom\n" {
		t.Fatalf("StripANSI = %q", got)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"id 550e8400-e29b-41d4-a716-446655440000 ok", "id <UUID> ok"},
		{"at 2025-11-03T10:52:43.804Z failed", "at <TS> failed"},
		{"at 2025-11-03 failed", "at <TS> failed"},
		{"time 10:52:43 tick", "time <TS> tick"},
		{"dial 192.168.1.10 refused", "dial <IP> refused"},
		{"pointer 0x7fff5fbff8ac dead", "pointer <ADDR> dead"},
		{"open /home/alice/proj/train.py failed", "open <PATH> failed"},
		{`open C:\Users\bob\app.py failed`, "open <PATH> failed"},
		{"No module named 'requests'", "No module named <VAL>"},
		{`key "secret-token" missing`, "key <VAL> missing"},
		{"line 42 col 7", "line <N> col <N>"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

const pyTraceA = `Traceback (most recent call last):
  File "/home/alice/proj/train.py", line 42, in <module>
    main()
  File "/home/alice/proj/train.py", line 17, in main
    run(123)
TypeError: unsupported operand type(s) for +: 'int' and 'str'
`

// Same error, different machine/time: path, line numbers, PID and quoted
// values all changed.
const pyTraceB = `2026-08-21 10:52:43 worker[1337] crashed
Traceback (most recent call last):
  File "/opt/services/api/train.py", line 7, in <module>
    main()
  File "/opt/services/api/train.py", line 3, in main
    run(999)
TypeError: unsupported operand type(s) for +: 'float' and 'list'
`

func TestPythonFingerprintStableAcrossPathLinePid(t *testing.T) {
	langA, sigA, fpA := Fingerprint(pyTraceA)
	langB, sigB, fpB := Fingerprint(pyTraceB)
	if langA != LangPython || langB != LangPython {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	if sigA != sigB {
		t.Fatalf("signatures differ:\nA=%q\nB=%q", sigA, sigB)
	}
	if fpA != fpB {
		t.Fatalf("fingerprints differ: %s vs %s (sig %q)", fpA, fpB, sigA)
	}
	want := "TypeError: unsupported operand type(s) for +: <VAL> and <VAL>"
	if sigA != want {
		t.Fatalf("signature = %q, want %q", sigA, want)
	}
}

const nodeErrA = `/home/alice/web/app.js:12
    throw new TypeError("user is undefined");
    ^

TypeError: Cannot read properties of undefined (reading 'name')
    at main (/home/alice/web/app.js:12:11)
    at Object.<anonymous> (/home/alice/web/app.js:20:1)
    at node:internal/modules/cjs/loader:1105:14
`

const nodeErrB = `TypeError: Cannot read properties of undefined (reading 'title')
    at main (/srv/app/build/index.js:3:5)
    at node:internal/modules/cjs/loader:999:10
`

func TestNodeFingerprintStableAcrossPaths(t *testing.T) {
	langA, sigA, fpA := Fingerprint(nodeErrA)
	langB, sigB, fpB := Fingerprint(nodeErrB)
	if langA != LangNode || langB != LangNode {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	// Messages differ in the quoted property only -> same template.
	if sigA != sigB || fpA != fpB {
		t.Fatalf("node mismatch:\nA=%q %s\nB=%q %s", sigA, fpA, sigB, fpB)
	}
}

func TestNodeModuleNotFound(t *testing.T) {
	raw := "node:internal/modules/cjs/loader:1042\n  throw err;\n  ^\n\n" +
		"Error: Cannot find module 'express'\nRequire stack:\n- /home/a/app/index.js\n" +
		"    at Module._resolveFilename (node:internal/modules/cjs/loader:1044:15)\n"
	lang, sig, fp := Fingerprint(raw)
	if lang != LangNode {
		t.Fatalf("lang = %q", lang)
	}
	if sig != "Error: Cannot find module <VAL>" {
		t.Fatalf("sig = %q", sig)
	}
	if fp == "" {
		t.Fatal("empty fingerprint")
	}
}

func TestDifferentErrorsDifferentFingerprints(t *testing.T) {
	a := "Traceback (most recent call last):\n  File \"/x/a.py\", line 1, in <module>\nTypeError: unsupported operand type(s) for +: 'int' and 'str'\n"
	b := "Traceback (most recent call last):\n  File \"/x/b.py\", line 1, in <module>\nKeyError: 'database_url'\n"
	_, _, fpA := Fingerprint(a)
	_, _, fpB := Fingerprint(b)
	if fpA == fpB {
		t.Fatal("different errors share a fingerprint")
	}
	ha, _ := strconv.ParseUint(fpA, 16, 64)
	hb, _ := strconv.ParseUint(fpB, 16, 64)
	if d := HammingDistance(ha, hb); d <= SimilarityThreshold {
		t.Fatalf("different errors too close: hamming=%d <= %d", d, SimilarityThreshold)
	}
}

func TestUnknownLanguageSkipped(t *testing.T) {
	// Output without any unambiguous error marker is skipped (the
	// marker-carrying cases live in generic_test.go's hit table).
	cases := []string{
		"Some random prose: with a colon, no traceback.\n",
		"warning: something mildly interesting\n",
		"",
	}
	for _, c := range cases {
		if lang, sig, fp := Fingerprint(c); sig != "" || fp != "" {
			t.Errorf("Fingerprint(%q) = (%q, %q, %q), want skipped", c, lang, sig, fp)
		}
	}
}

func TestSimHashDeterministic(t *testing.T) {
	s := "TypeError: unsupported operand type(s) for +: <VAL> and <VAL>"
	if SimHash(s) != SimHash(s) {
		t.Fatal("SimHash not deterministic")
	}
	if SimHash(s) == SimHash("KeyError: <VAL>") {
		t.Fatal("distinct signatures collided")
	}
}

func TestHammingDistance(t *testing.T) {
	if d := HammingDistance(0, 0); d != 0 {
		t.Fatalf("d(0,0)=%d", d)
	}
	if d := HammingDistance(0, 1); d != 1 {
		t.Fatalf("d(0,1)=%d", d)
	}
	if d := HammingDistance(0, ^uint64(0)); d != 64 {
		t.Fatalf("d(0,~0)=%d", d)
	}
}

func TestPythonChainedTracebackTakesLastException(t *testing.T) {
	raw := `Traceback (most recent call last):
  File "/x/a.py", line 1, in <module>
    connect()
ConnectionRefusedError: [Errno 111] Connection refused

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "/x/b.py", line 2, in <module>
    main()
RuntimeError: failed to start worker 12
`
	lang, sig, _ := Fingerprint(raw)
	if lang != LangPython {
		t.Fatalf("lang = %q", lang)
	}
	if !strings.HasPrefix(sig, "RuntimeError:") {
		t.Fatalf("sig = %q, want final RuntimeError", sig)
	}
}

func TestRegisterLanguageExtractor(t *testing.T) { // A new language plugs in with one file and one Register call. The
	// marker is deliberately unique so the registration cannot disturb
	// the other tests' inputs.
	Register("fake", func(text string, _ map[string]bool) (string, bool) {
		if strings.Contains(text, "FAKE-MAGIC") {
			return "FakeError: something", true
		}
		return "", false
	})

	lang, sig, fp := Fingerprint("blah\nFAKE-MAGIC\nblah\n")
	if lang != "fake" || sig != "FakeError: something" || fp == "" {
		t.Fatalf("registered extractor: lang=%q sig=%q fp=%q", lang, sig, fp)
	}

	// Registered last = probed last: Python/Node still claim their own.
	lang, _, _ = Fingerprint(pyTraceA)
	if lang != LangPython {
		t.Fatalf("python trace misrouted to %q", lang)
	}
}

// SyntaxError-family output has no "Traceback" header.
const pySyntaxErrA = `  File "/home/alice/proj/demo2.py", line 3
    if i > 5
            ^
SyntaxError: expected ':'
`

// Same mistake, another machine: path, line number and caret column differ.
const pySyntaxErrB = `  File "/opt/svc/jobs/report.py", line 88
    if count > 10
                ^
SyntaxError: expected ':'
`

func TestPythonSyntaxErrorWithoutTraceback(t *testing.T) {
	langA, sigA, fpA := Fingerprint(pySyntaxErrA)
	langB, sigB, fpB := Fingerprint(pySyntaxErrB)
	if langA != LangPython || langB != LangPython {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	if sigA != sigB || fpA != fpB {
		t.Fatalf("syntax error fingerprint not stable:\nA=%q %s\nB=%q %s", sigA, fpA, sigB, fpB)
	}
	if sigA != "SyntaxError: expected <VAL>" {
		t.Fatalf("sig = %q", sigA)
	}
}

func TestPythonNoTracebackRejectsProse(t *testing.T) {
	// Without a traceback header only suffix-typed names may match:
	// "Note: ..." or "hint: ..." prose must not become an error.
	cases := []string{
		"Note: check your config\nHint: try again\n",
		"warning: low disk space\n", // lowercase 'warning' has no suffix match
		"Some prose: with colon\n",
	}
	for _, c := range cases {
		if sig, ok := pythonSignature(StripANSI(c), nil); ok {
			t.Errorf("pythonSignature(%q) = %q, want skip", c, sig)
		}
	}
}

func TestPythonTracebackPathUnchanged(t *testing.T) {
	// With the marker present, non-suffixed dotted names still qualify
	// (e.g. django.core.exceptions.ImproperlyConfigured).
	raw := "Traceback (most recent call last):\n  File \"/x.py\", line 1, in <module>\ndjango.core.exceptions.ImproperlyConfigured: settings broken\n"
	lang, sig, _ := Fingerprint(raw)
	if lang != LangPython || sig != "django.core.exceptions.ImproperlyConfigured: settings broken" {
		t.Fatalf("lang=%q sig=%q", lang, sig)
	}
}
