package cli

import (
	"strings"
	"testing"
)

func TestConfirmAnswer(t *testing.T) {
	cases := []struct {
		input      string
		requireYes bool
		want       bool
	}{
		{"y\n", false, true},
		{"YES\n", false, true},
		{"yes\n", false, true},
		{"\n", false, false},   // empty = no (default safe)
		{"n\n", false, false},  // explicit no
		{"ye\n", false, false}, // near-miss is a no
		{"yes\n", true, true},
		{"y\n", true, false}, // clear never accepts "y"
		{"Yes\n", true, false},
		{"\n", true, false},
	}
	for _, c := range cases {
		got, err := confirmAnswer(strings.NewReader(c.input), &strings.Builder{}, "sure? ", c.requireYes, true)
		if err != nil || got != c.want {
			t.Errorf("confirmAnswer(%q, requireYes=%v) = %v, %v; want %v",
				c.input, c.requireYes, got, err, c.want)
		}
	}
}

func TestConfirmAnswerNonTTYRefuses(t *testing.T) {
	ok, err := confirmAnswer(strings.NewReader("yes\n"), &strings.Builder{}, "sure? ", false, false)
	if err == nil || ok {
		t.Fatalf("non-TTY must refuse: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("refusal should point at --yes: %v", err)
	}
}
