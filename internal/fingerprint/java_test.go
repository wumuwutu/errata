package fingerprint

import (
	"strings"
	"testing"
)

const javaNpeA = `Exception in thread "main" java.lang.NullPointerException: Cannot read field "timeout" because "config" is null
	at com.app.Main.main(Main.java:12)
`

// Same error, another machine: thread name, class paths and line numbers
// all drift.
const javaNpeB = `Exception in thread "worker-3" java.lang.NullPointerException: Cannot read field "timeout" because "config" is null
	at com.app.Worker.run(Worker.java:88)
	at java.base/java.lang.Thread.run(Thread.java:840)
`

func TestJavaExceptionInThread(t *testing.T) {
	langA, sigA, fpA := Fingerprint(javaNpeA)
	langB, sigB, fpB := Fingerprint(javaNpeB)
	if langA != LangJava || langB != LangJava {
		t.Fatalf("langs: %q %q", langA, langB)
	}
	want := `java.lang.NullPointerException: Cannot read field "timeout" because "config" is null`
	if sigA != want {
		t.Fatalf("sig = %q, want %q", sigA, want)
	}
	if sigA != sigB || fpA != fpB {
		t.Fatalf("fingerprint not stable:\nA=%q %s\nB=%q %s", sigA, fpA, sigB, fpB)
	}
}

// Message-less exception (a bare NPE on older JVMs): the top frame carries
// the identity, with its line number normalized away.
func TestJavaBareExceptionUsesTopFrame(t *testing.T) {
	a := "Exception in thread \"main\" java.lang.NullPointerException\n\tat com.app.Main.run(Main.java:10)\n\tat com.app.Main.main(Main.java:5)\n"
	b := "Exception in thread \"pool-2-thread-1\" java.lang.NullPointerException\n\tat com.app.Main.run(Main.java:33)\n"
	lang, sigA, fpA := Fingerprint(a)
	_, sigB, fpB := Fingerprint(b)
	if lang != LangJava {
		t.Fatalf("lang = %q", lang)
	}
	want := "java.lang.NullPointerException: at com.app.Main.run(Main.java:<N>)"
	if sigA != want {
		t.Fatalf("sig = %q, want %q", sigA, want)
	}
	if sigA != sigB || fpA != fpB {
		t.Fatalf("line drift moved the fingerprint: %q vs %q", sigA, sigB)
	}

	// A different top frame is a different error.
	c := "Exception in thread \"main\" java.lang.NullPointerException\n\tat com.app.Other.run(Other.java:10)\n"
	if _, _, fpC := Fingerprint(c); fpC == fpA {
		t.Fatal("different top frames share a fingerprint")
	}
}

func TestJavaRejectsUndottedClass(t *testing.T) {
	// A bare word after the marker is not a qualified exception class; the
	// precise extractor must abstain (the generic fallback claims it as
	// unknown instead).
	if sig, ok := javaSignature(`Exception in thread "main" NullPointerException: boom`, nil); ok {
		t.Fatalf("javaSignature accepted undotted class: %q", sig)
	}
	if !strings.Contains(javaExcRe.String(), "Exception in thread") {
		t.Fatal("marker drifted")
	}
}
