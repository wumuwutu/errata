package redact

import "testing"

func TestRedactRules(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"url credentials", "dial https://alice:s3cret@db.internal:5432/app failed",
			"dial https://alice:***@db.internal:5432/app failed"},
		{"url without credentials untouched", "dial https://db.internal:5432/app failed",
			"dial https://db.internal:5432/app failed"},
		{"password=", "connect failed: password=hunter2 retry",
			"connect failed: password=*** retry"},
		{"passwd with space", `passwd: "qwerty 123"`,
			`passwd: "***"`},
		{"authorization bearer", "Authorization: Bearer eyJrandomtoken123 denied",
			"Authorization: *** denied"},
		{"token colon", "AUTH_TOKEN: abcdef123456 expired",
			"AUTH_TOKEN: *** expired"},
		{"api_key", "api_key=xyz123 rejected", "api_key=*** rejected"},
		{"apiKey camel", "apiKey=xyz123 rejected", "apiKey=*** rejected"},
		{"secret case-insensitive", "SECRET=s3cret ok", "SECRET=*** ok"},
		{"github pat classic", "fatal: could not read: ghp_0123456789abcdefABCDEF01234567",
			"fatal: could not read: ***"},
		{"github oauth", "token gho_0123456789abcdefABCDEF01234567 used",
			"token *** used"},
		{"github fine-grained", "github_pat_11ABCDEFG0ZrqaWv0123456789abcdefghijklm invalid",
			"*** invalid"},
		{"openai style", "invalid key sk-abcdefghABCDEFGH12345678 provided",
			"invalid key *** provided"},
		{"aws access key", "denied for AKIAIOSFODNN7EXAMPLE on bucket",
			"denied for *** on bucket"},
		{"slack bot", "auth failed: xoxb-123456789012-abcdefghijkl",
			"auth failed: ***"},
		{"jwt", "bad token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			"bad token ***"},
	}
	for _, c := range cases {
		if got := String(c.in); got != c.want {
			t.Errorf("%s: String(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestRedactPassesCleanOutputThrough(t *testing.T) {
	cases := []string{
		"Traceback (most recent call last):\n  File \"/x/a.py\", line 1, in <module>\nTypeError: boom\n",
		"panic: runtime error: index out of range [3] with length 3\n",
		"main.c:12:5: error: 'foo' undeclared (first use in this function)\n",
		"the password is wrong, try again", // prose: no key=value shape
		"secretary: hugo",                 // ends with a secret name but isn't one
		"bash: cd: dsad: 没有那个文件或目录\n",
		"",
	}
	for _, c := range cases {
		if got := String(c); got != c {
			t.Errorf("clean output modified:\nin   %q\ngot  %q", c, got)
		}
	}
}
