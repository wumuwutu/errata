#!/usr/bin/env bash
# Integration test for the zsh shell hook (driver is bash; the session
# under test runs zsh).
# Usage: hook-it.zsh <path-to-err-binary>
set -u

ERR="$1"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/errata-hooktest-zsh.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
export XDG_DATA_HOME="$TMP/data"
export XDG_CONFIG_HOME="$TMP/conf"

fails=0
check() {
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "ok   - $name"
  else
    echo "FAIL - $name"
    fails=$((fails + 1))
  fi
}

cat > "$TMP/fail.py" <<'PY'
import os
if not os.environ.get("FIXED"):
    raise TypeError("unsupported operand type(s) for +: 'int' and 'str'")
PY

# PATH shim that logs every hook-driven err invocation, then forwards to
# the real binary. The zshrc puts the shim first on PATH; its own
# `err init` call uses the absolute path and bypasses the shim.
SHIM_LOG="$TMP/shim.log"
mkdir -p "$TMP/shim"
cat > "$TMP/shim/err" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >> '$SHIM_LOG'
exec '$ERR' "\$@"
EOF
chmod +x "$TMP/shim/err"

# Isolated ZDOTDIR: .zshrc contains only the hook (with shimmed err on PATH).
mkdir -p "$TMP/zdot"
cat > "$TMP/zdot/.zshrc" <<EOF
export XDG_DATA_HOME='$XDG_DATA_HOME'
export XDG_CONFIG_HOME='$XDG_CONFIG_HOME'
export PATH='$TMP/shim:$(dirname "$ERR")':"\$PATH"
eval "\$('$ERR' init zsh)"
EOF

# Since v0.1.12 the success path only calls err when this session has seen
# a failure (one check per failure, then the gate closes again), so every
# success meant to exercise solvedHint is preceded by a failure: `false`
# re-arms the gate without recording anything (it fails with no stderr).
printf '%s\n' \
  'false; echo "EC=$?"' \
  "python3 \"$TMP/fail.py\"" \
  'true' \
  "python3 -c 'print(1)'" \
  "python3 \"$TMP/fail.py\"" \
  'false' \
  "python3 -c 'print(1)'" \
  'false' \
  "FIXED=1 python3 \"$TMP/fail.py\"" \
  'echo SESSION-DONE' \
  | ZDOTDIR="$TMP/zdot" zsh -i >"$TMP/out.txt" 2>&1

# 1. Exit codes pass through the hook untouched.
check "exit code transparent" grep -q 'EC=1' "$TMP/out.txt"
# 2. The failing python command was recorded.
"$ERR" show 1 >"$TMP/show1.txt" 2>&1
check "error recorded" grep -q 'TypeError' "$TMP/show1.txt"
# 3. Same error twice -> one record, seen twice (not two records).
check "reoccurrence counted" grep -q '2 times' "$TMP/show1.txt"
if "$ERR" show 2 >/dev/null 2>&1; then
  echo "FAIL - success command not recorded (found error #2)"
  fails=$((fails + 1))
else
  echo "ok   - success command not recorded"
fi
# 4. Second occurrence produced the gray hit hint.
check "hit hint shown" grep -q 'occurrence #2' "$TMP/out.txt"
# 5. Session actually completed (hook did not hang the shell).
check "session completed" grep -q 'SESSION-DONE' "$TMP/out.txt"
# 6. Success detection: a success nudges only when the gate is armed (a
#    failure seen earlier in this session) AND it shares the program AND
#    a target argument (the script) with the pending error: 'true' and
#    `python3 -c 'print(1)'` (same program, other target) stay quiet even
#    with the gate armed, while the FIXED=1 re-run of the same script
#    nudges — exactly once.
check "success hint shown" grep -q 'looks fixed' "$TMP/out.txt"
hint_count=$(grep -c 'looks fixed' "$TMP/out.txt")
check "success hint not repeated" test "$hint_count" -eq 1
# 6b. Hint style: ASCII dashes, looks-fixed split over two lines.
check "hint uses new prefix" grep -q -- '--err-- seen' "$TMP/out.txt"
check "no box-dash hint" bash -c "! grep -q '── err' '$TMP/out.txt'"
check "looks-fixed is two lines" bash -c "! grep -q 'looks fixed.*err fix' '$TMP/out.txt'"
# 6c. Command attribution: the recorded command is the one that failed.
check "command attributed to failing python" grep -q "command:.*python3 .*fail.py" "$TMP/show1.txt"

# 7. err's own output must never pollute a later capture: err pending
#    prints to stderr (into the session buffer); the next failure's record
#    must carry only its own stderr and command. (The preexec sentinel
#    lands behind those bytes in the tee pipe, so they are cut away.)
cat > "$TMP/fail3.py" <<'PY'
raise ValueError("zz-pollution-probe")
PY
printf '%s\n' \
  "err pending" \
  "python3 \"$TMP/fail3.py\"" \
  'echo POLLUTE-DONE' \
  | ZDOTDIR="$TMP/zdot" zsh -i >"$TMP/out4.txt" 2>&1
newid=$("$ERR" pending 2>/dev/null | grep 'zz-pollution-probe' | awk '{print $1}')
check "pollution probe recorded" test -n "$newid"
"$ERR" show "$newid" >"$TMP/shown.txt" 2>&1
check "signature unpolluted by err output" grep -qE '^signature: +ValueError: zz-pollution-probe$' "$TMP/shown.txt"
check "command kept after err pending ran" grep -q "command:.*python3 .*fail3.py" "$TMP/shown.txt"

# 8. Flag fidelity: a failing command's flags must survive verbatim into
#    errors.command (an old hook once recorded 'rmdir -f tmp' as
#    'rmdir tmp'). A missing-script python run exits 2 with a
#    recognizable "can't open file" error.
printf '%s\n' \
  "python3 -u \"$TMP/no_such_script_xyz.py\"" \
  'echo FLAG-DONE' \
  | ZDOTDIR="$TMP/zdot" zsh -i >"$TMP/out5.txt" 2>&1
fid=$("$ERR" pending --all 2>/dev/null | grep "can't open file" | awk '{print $1}')
check "flag command recorded" test -n "$fid"
"$ERR" show "$fid" >"$TMP/showf.txt" 2>&1
check "flag preserved in command" grep -q "command:.*python3 -u .*no_such_script_xyz.py" "$TMP/showf.txt"

# 9. Zero-subprocess success path: a session with NO failing command must
#    not spawn err from the prompt path at all — even though the database
#    (filled by the sessions above) has plenty of pending errors. This is
#    the regression behind the multi-second prompt on WSL, where every
#    err exec pays a full Defender scan.
: > "$SHIM_LOG"
printf '%s\n' \
  'true' \
  'echo hello' \
  'ls >/dev/null' \
  'echo QUIET-DONE' \
  | ZDOTDIR="$TMP/zdot" zsh -i >"$TMP/out6.txt" 2>&1
check "success-only session completed" grep -q 'QUIET-DONE' "$TMP/out6.txt"
check "no err process on success-only prompts" bash -c "! grep -q hook-event '$SHIM_LOG'"

# 10. The gate opens on failure and closes after one success check:
#     fail, then two successes -> exactly one success-path hook-event.
: > "$SHIM_LOG"
printf '%s\n' \
  "python3 \"$TMP/fail.py\"" \
  'true' \
  'true' \
  'echo GATE-DONE' \
  | ZDOTDIR="$TMP/zdot" zsh -i >"$TMP/out7.txt" 2>&1
check "gate session completed" grep -q 'GATE-DONE' "$TMP/out7.txt"
gate_count=$(grep -c 'hook-event --exit-code 0' "$SHIM_LOG" || true)
check "one success check per failure" test "$gate_count" -eq 1

if [ "$fails" -gt 0 ]; then
  echo "---"
  echo "$fails check(s) FAILED; session output:"
  cat "$TMP/out.txt"
  exit 1
fi
echo "all zsh hook checks passed"
