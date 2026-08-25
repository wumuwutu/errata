package fingerprint

import "testing"

const goPanicA = `panic: runtime error: index out of range [3] with length 3

goroutine 1 [running]:
main.main()
	/home/alice/proj/main.go:12 +0x1f
exit status 2
`

// Same bug elsewhere: index values, goroutine dump and paths drift.
const goPanicB = `panic: runtime error: index out of range [7] with length 8

goroutine 5 [running]:
main.worker(0x0)
	/srv/app/worker.go:44 +0x2a
created by main.main
	/srv/app/main.go:9 +0x55
`

func TestGoPanicStableAcrossStackAndPaths(t *testing.T) {
	langA, sigA, fpA := Fingerprint(goPanicA)
	langB, sigB, fpB := Fingerprint(goPanicB)
	if langA != LangGo || langB != LangGo {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	want := "panic: runtime error: index out of range [<N>] with length <N>"
	if sigA != want {
		t.Fatalf("sig = %q, want %q", sigA, want)
	}
	if sigA != sigB || fpA != fpB {
		t.Fatalf("fingerprint not stable:\nA=%q %s\nB=%q %s", sigA, fpA, sigB, fpB)
	}
}

func TestGoCompileError(t *testing.T) {
	a := "# command-line-arguments\n./main.go:12:3: syntax error: unexpected newline, expecting comma or }\n"
	b := "# example.com/proj\n./cmd/app/main.go:47:9: syntax error: unexpected newline, expecting comma or }\n"
	langA, sigA, fpA := Fingerprint(a)
	langB, sigB, fpB := Fingerprint(b)
	if langA != LangGo || langB != LangGo {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	want := "syntax error: unexpected newline, expecting comma or }"
	if sigA != want {
		t.Fatalf("sig = %q, want %q", sigA, want)
	}
	if sigA != sigB || fpA != fpB {
		t.Fatalf("file/line drift moved the fingerprint: %q vs %q", sigA, sigB)
	}

	// A different message is a different error.
	c := "./main.go:5:2: undefined: fmt\n"
	if _, _, fpC := Fingerprint(c); fpC == fpA {
		t.Fatal("different compile errors share a fingerprint")
	}
}

func TestGoRejectsGccAndProse(t *testing.T) {
	cases := []string{
		"main.c:12:5: error: 'foo' undeclared (first use in this function)\n", // gcc, not go
		"main.go:12:5: note: something informational\n",                       // no error message shape
		"Everything went smoothly\n",
	}
	for _, c := range cases {
		if sig, ok := goSignature(c, nil); ok {
			t.Errorf("goSignature(%q) = %q, want skip", c, sig)
		}
	}
}
