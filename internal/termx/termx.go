// Package termx holds terminal-display helpers shared by the CLI and the
// hint renderer: a tiny NO_COLOR-aware ANSI palette, display-width-safe
// truncation (never splits a rune, CJK counts double), and ~/ shortening.
package termx

import (
	"io"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// Palette. faint is the base color of every errata notice (kept visually
// distinct from the user's own terminal text); cyan marks command names;
// green marks the "looks fixed" keyword; bright marks key payloads like
// the error signature or the recorded solution.
//
// 想自己改颜色？只改下面这几个值即可，所有提示颜色都从这里取。
// 常用 ANSI 色码速查（\x1b[<code>m）：
//
//	30 黑   31 红   32 绿   33 黄   34 蓝   35 品红  36 青   37 白
//	90 亮黑(灰)  91 亮红  92 亮绿  93 亮黄  94 亮蓝  95 亮品红  96 亮青  97 亮白
//	0 重置（reset，勿动）
//
// 红色（red/brightRed）只用于破坏性命令的确认提示（err delete/clear），
// 日常提示绝不用红。改完跑 go test ./internal/termx/ 确认没破坏 NO_COLOR 不变量。
const (
	faint     = "\x1b[90m"
	cyan      = "\x1b[36m"
	green     = "\x1b[92m"
	bright    = "\x1b[97m"
	red       = "\x1b[31m"
	brightRed = "\x1b[91m"
	reset     = "\x1b[0m"
)

// NoColor disables all palette output. Initialized from the NO_COLOR
// environment variable (https://no-color.org); exported so tests can flip
// it directly.
var NoColor = os.Getenv("NO_COLOR") != ""

func paint(color, s string) string {
	if NoColor || s == "" {
		return s
	}
	return color + s + reset
}

// Faint paints s in the base faint gray (the default for errata text).
func Faint(s string) string { return paint(faint, s) }

// Cyan paints a command name (err fix / err show / err pending).
func Cyan(s string) string { return paint(cyan, s) }

// Green paints the "looks fixed" keyword in bright green.
func Green(s string) string { return paint(green, s) }

// Bright paints key payloads (error signature, solution) in bright white.
func Bright(s string) string { return paint(bright, s) }

// Red paints in plain red (reserved for warnings that should not shout).
func Red(s string) string { return paint(red, s) }

// BrightRed paints in bright red: ONLY for the confirmation prompt of
// destructive commands (err delete / err clear) — never for notices.
func BrightRed(s string) string { return paint(brightRed, s) }

// PlainUnlessTTY forces plain (uncolored) output when w is not a terminal
// and returns a restore func. Interactive CLI commands defer it so piped
// output never carries ANSI escapes; the hook/hint path is always
// terminal-bound and never calls it.
func PlainUnlessTTY(w io.Writer) func() {
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return func() {}
	}
	old := NoColor
	NoColor = true
	return func() { NoColor = old }
}

// PadRight pads s with spaces to cols display cells (CJK-aware). Strings
// already wider than cols are returned unchanged.
func PadRight(s string, cols int) string {
	return runewidth.FillRight(s, cols)
}

// Truncate shortens s to at most maxCols display columns, appending an
// ellipsis when cut. Width is measured in terminal cells (CJK and other
// wide runes count double); a rune is never split. maxCols <= 0 returns s
// unchanged.
func Truncate(s string, maxCols int) string {
	if maxCols <= 0 || runewidth.StringWidth(s) <= maxCols {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxCols-1 { // reserve one cell for the ellipsis
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}

// ShortenHome rewrites an absolute path under the user's home as ~/…
func ShortenHome(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if strings.HasPrefix(dir, home+string(os.PathSeparator)) {
		return "~" + dir[len(home):]
	}
	return dir
}
