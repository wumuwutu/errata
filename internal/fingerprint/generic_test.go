package fingerprint

import "testing"

func TestGenericFallbackHits(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // normalized signature
	}{
		{"gcc tool-prefixed", "gcc: error: unrecognized command-line option '-x'\n",
			"gcc: error: unrecognized command-line option '-x'"},
		{"shell command not found", "bash[1234]: pyhton: command not found\n",
			"bash[<N>]: pyhton: command not found"},
		{"permission denied", "bash: /etc/shadow: Permission denied\n",
			"bash: <PATH>: Permission denied"},
		{"no such file", "cat: /nope.txt: No such file or directory\n",
			"cat: <PATH>: No such file or directory"},
		{"git fatal", "fatal: not a git repository (or any of the parent directories): .git\n",
			"fatal: not a git repository (or any of the parent directories): .git"},
		{"node error without frames", "Error: connect ECONNREFUSED 127.0.0.1:5432\n",
			"Error: connect ECONNREFUSED <IP>:<N>"},
		// Localized shell builtin errors: no English keyword, matched by
		// shape (shell: builtin: message); the operand is blanked.
		{"shell builtin zh cd", "bash: cd: dsad: 没有那个文件或目录\n",
			"bash: cd: <ARG>: 没有那个文件或目录"},
		{"shell builtin zh zsh", "zsh: cd: projets: 没有那个文件或目录\n",
			"zsh: cd: <ARG>: 没有那个文件或目录"},
		{"shell builtin no operand", "bash: cd: too many arguments\n",
			"bash: cd: too many arguments"},
		// The English keyword markers keep winning when both match, so
		// existing signatures do not move.
		{"shell builtin english keeps keyword signature", "bash: cd: /nope: No such file or directory\n",
			"bash: cd: <PATH>: No such file or directory"},
		// Ubuntu/Debian command-not-found helper output (two lines; the
		// "sudo apt install ..." suggestion line must not be picked).
		{"ubuntu command-not-found helper", "Command 'pip' not found, but can be installed with:\nsudo apt install python3-pip\n",
			"Command 'pip' not found, but can be installed with:"},
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
		// Prose near the shell-builtin shape must not match: not anchored
		// at the shell name, or no "word: " right after "<shell>: ".
		"see the manual: bash: cd changes directory\n",
		"bash: startup files are read at login\n",
		"zsh: themes and plugins: a field guide\n",
		"bash: cd\n",
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
	cases := []struct {
		raw  string
		want string
	}{
		{pyTraceA, LangPython},
		{nodeErrA, LangNode},
		{javaNpeA, LangJava},
		{goPanicA, LangGo},
		{gccErrA, LangC},
	}
	for _, c := range cases {
		if lang, _, _ := Fingerprint(c.raw); lang != c.want {
			t.Errorf("claimed by %q, want %q", lang, c.want)
		}
	}
}

// TestShellBuiltinLocalizedStable reproduces the zh_CN report: a builtin
// error under a non-English locale must be recorded, and only the volatile
// operand (the directory the user typed) may vary — the fingerprint must
// not.
func TestShellBuiltinLocalizedStable(t *testing.T) {
	lang1, sig1, fp1 := Fingerprint("bash: cd: dsad: 没有那个文件或目录\n")
	lang2, sig2, fp2 := Fingerprint("bash: cd: some_other_dir: 没有那个文件或目录\n")
	if fp1 == "" || fp2 == "" {
		t.Fatalf("localized shell builtin error must be recorded: %q / %q", sig1, sig2)
	}
	if lang1 != LangUnknown || lang2 != LangUnknown {
		t.Fatalf("lang = %q/%q, want unknown", lang1, lang2)
	}
	if sig1 != sig2 || fp1 != fp2 {
		t.Fatalf("operand must not move the fingerprint: %q vs %q", sig1, sig2)
	}

	// A different builtin or a different message is a different error.
	if _, _, fp3 := Fingerprint("zsh: cd: dsad: 没有那个文件或目录\n"); fp3 == fp1 {
		t.Fatal("different shell must not share the fingerprint")
	}
	if _, _, fp4 := Fingerprint("bash: cd: dsad: 权限不够\n"); fp4 == fp1 {
		t.Fatal("different message must not share the fingerprint")
	}
}
