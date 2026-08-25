package fingerprint

import "testing"

const gccErrA = `main.c:12:5: error: 'foo' undeclared (first use in this function)
   12 |     foo();
      |     ^~~
`

// Same error, another checkout: path, line and column drift.
const gccErrB = `/home/alice/proj/main.c:47:2: error: 'foo' undeclared (first use in this function)
`

func TestCGccErrorStableAcrossFileLine(t *testing.T) {
	langA, sigA, fpA := Fingerprint(gccErrA)
	langB, sigB, fpB := Fingerprint(gccErrB)
	if langA != LangC || langB != LangC {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	want := "error: 'foo' undeclared (first use in this function)"
	if sigA != want {
		t.Fatalf("sig = %q, want %q", sigA, want)
	}
	if sigA != sigB || fpA != fpB {
		t.Fatalf("fingerprint not stable:\nA=%q %s\nB=%q %s", sigA, fpA, sigB, fpB)
	}
}

func TestCClangFatalError(t *testing.T) {
	a := "src/app.cpp:8:10: fatal error: 'boost/asio.hpp' file not found\n#include <boost/asio.hpp>\n         ^~~~~~~~~~~~~~~~~\n"
	b := "/opt/build/src/net/app.cpp:23:10: fatal error: 'boost/asio.hpp' file not found\n"
	langA, sigA, fpA := Fingerprint(a)
	langB, sigB, fpB := Fingerprint(b)
	if langA != LangC || langB != LangC {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	want := "error: 'boost/asio.hpp' file not found"
	if sigA != want {
		t.Fatalf("sig = %q, want %q", sigA, want)
	}
	if sigA != sigB || fpA != fpB {
		t.Fatalf("path drift moved the fingerprint: %q vs %q", sigA, sigB)
	}
}

func TestCRejectsWarningsAndNotes(t *testing.T) {
	cases := []string{
		"main.c:10:6: warning: unused variable 'x'\n",
		"main.c:10:6: note: declared here\n",
		"gcc: error: unrecognized command-line option '-x'\n", // no file:line — generic's job
	}
	for _, c := range cases {
		if sig, ok := cSignature(c, nil); ok {
			t.Errorf("cSignature(%q) = %q, want skip", c, sig)
		}
	}
}
