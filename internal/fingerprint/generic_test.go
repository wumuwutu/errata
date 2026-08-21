package fingerprint

import "testing"

func TestGenericFallbackHits(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // normalized signature
	}{
		{"java npe", "Exception in thread \"main\" java.lang.NullPointerException: Cannot read field \"x\" because \"cfg\" is null\n\tat com.app.Main.main(Main.java:12)\n",
			"Exception in thread <THREAD> java.lang.NullPointerException: Cannot read field \"x\" because \"cfg\" is null"},
		{"gcc", "main.c:12:5: error: 'foo' undeclared (first use in this function)\n",
			"main.c:<N>:<N>: error: 'foo' undeclared (first use in this function)"},
		{"gcc tool-prefixed", "gcc: error: unrecognized command-line option '-x'\n",
			"gcc: error: unrecognized command-line option '-x'"},
		{"shell command not found", "bash[1234]: pyhton: command not found\n",
			"bash[<N>]: pyhton: command not found"},
		{"permission denied", "bash: /etc/shadow: Permission denied\n",
			"bash: <PATH>: Permission denied"},
		{"no such file", "cat: /nope.txt: No such file or directory\n",
			"cat: <PATH>: No such file or directory"},
		{"go panic", "panic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\n",
			"panic: runtime error: index out of range [<N>] with length <N>"},
		{"git fatal", "fatal: not a git repository (or any of the parent directories): .git\n",
			"fatal: not a git repository (or any of the parent directories): .git"},
		{"node error without frames", "Error: connect ECONNREFUSED 127.0.0.1:5432\n",
			"Error: connect ECONNREFUSED <IP>:<N>"},
	}
	for _, c := range cases {
		lang, sig, fp := Fingerprint(c.raw)
		if lang != LangUnknown {
			t.Errorf("%s: lang = %q, want unknown", c.name, lang)
		}
		if sig != c.want {
			t.Errorf("%s: sig = %q, want %q", c.name, sig, c.want)
		}
		if fp == "" {
			t.Errorf("%s: empty fingerprint", c.name)
		}
	}
}

func TestGenericFallbackRejects(t *testing.T) {
	cases := []string{
		"2026-08-21 10:00:00 INFO  starting server on :8080\n",
		"Downloading packages... 45%\n",
		"Everything went smoothly: no issues at all\n",
		"main.c:10:6: note: declared here\n",      // gcc note, not error
		"main.c:10:6: warning: unused variable\n", // gcc warning, not error
		"level=info msg=\"all good\"\n",           // structured log
		"Some random prose: with a colon\n",
		"",
	}
	for _, c := range cases {
		if _, sig, fp := Fingerprint(c); sig != "" || fp != "" {
			t.Errorf("Fingerprint(%q) = (%q, %q), want skip", c, sig, fp)
		}
	}
}

func TestGenericRegisteredLast(t *testing.T) {
	// Language extractors claim their own before the fallback runs.
	if lang, _, _ := Fingerprint(pyTraceA); lang != LangPython {
		t.Fatalf("python trace claimed by %q", lang)
	}
	if lang, _, _ := Fingerprint(nodeErrA); lang != LangNode {
		t.Fatalf("node error claimed by %q", lang)
	}
}
