#!/usr/bin/env bash
# Integration test for the bash shell hook.
# Usage: hook-it.bash <path-to-err-binary>
set -u

ERR="$1"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/dejavu-hooktest-bash.XXXXXX")
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

# Isolated rcfile: just the hook (with the err binary on PATH).
cat > "$TMP/rc" <<EOF
export XDG_DATA_HOME='$XDG_DATA_HOME'
export XDG_CONFIG_HOME='$XDG_CONFIG_HOME'
export PATH='$(dirname "$ERR")':"\$PATH"
eval "\$('$ERR' init bash)"
EOF

# Drive an interactive bash: failing command, success, same failure again.
printf '%s\n' \
  'false; echo "EC=$?"' \
  "python3 \"$TMP/fail.py\"" \
  'true' \
  "python3 -c 'print(1)'" \
  "FIXED=1 python3 \"$TMP/fail.py\"" \
  "python3 \"$TMP/fail.py\"" \
  'echo SESSION-DONE' \
  | bash --rcfile "$TMP/rc" -i >"$TMP/out.txt" 2>&1

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
# 6. Success detection: a success nudges only when it shares the program
#    AND a target argument (the script) with the pending error: 'true' and
#    `python3 -c 'print(1)'` (same program, other target) stay quiet, while
#    the FIXED=1 re-run of the same script nudges — exactly once.
check "success hint shown" grep -q 'looks fixed' "$TMP/out.txt"
hint_count=$(grep -c 'looks fixed' "$TMP/out.txt")
check "success hint not repeated" test "$hint_count" -eq 1
# 6b. Hint style: ASCII dashes, looks-fixed split over two lines, the
#     second line starting at column 0.
check "hint uses new prefix" grep -q -- '--err-- seen' "$TMP/out.txt"
check "no box-dash hint" bash -c "! grep -q '── err' '$TMP/out.txt'"
check "looks-fixed is two lines" bash -c "! grep -q 'looks fixed.*err fix' '$TMP/out.txt'"
check "looks-fixed line 2 present" grep -q 'err fix.*to record the solution' "$TMP/out.txt"
# 6c. Command attribution: the recorded command is the one that failed.
check "command attributed to failing python" grep -q "command:.*python3 .*fail.py" "$TMP/show1.txt"
# 7. The ignore blacklist applies on the hook path too.
"$ERR" ignore add --command python3 >/dev/null 2>&1
printf '%s\n' \
  "python3 \"$TMP/fail.py\"" \
  'echo IGNORE-CHECK-DONE' \
  | bash --rcfile "$TMP/rc" -i >"$TMP/out2.txt" 2>&1
"$ERR" show 1 >"$TMP/show1b.txt" 2>&1
check "ignore respected on hook path" grep -q '2 times' "$TMP/show1b.txt"
"$ERR" ignore remove --command python3 >/dev/null 2>&1
# 8. Program gating in isolation: a fresh error, then only an unrelated
#    program succeeds — no nudge (remind-once cannot mask this: this
#    error was never reminded).
cat > "$TMP/fail2.py" <<'PY'
raise KeyError("missing_key")
PY
printf '%s\n' \
  "python3 \"$TMP/fail2.py\"" \
  "node -e 'console.log(1)'" \
  'echo UNRELATED-DONE' \
  | bash --rcfile "$TMP/rc" -i >"$TMP/out3.txt" 2>&1
check "unrelated success stays quiet" bash -c "! grep -q 'looks fixed' '$TMP/out3.txt'"
# 9. A second distinct failure keeps its own command (no cross-command
#    attribution even when errors interleave with other commands).
"$ERR" show 2 >"$TMP/show2.txt" 2>&1
check "earlier error kept its command" grep -q "command:.*python3 .*fail2.py" "$TMP/show2.txt"
# 10. err's own output must never pollute a later capture: err pending
#     prints to stderr (into the session buffer); the next failure's record
#     must carry only its own stderr and command. (The preexec sentinel
#     lands behind those bytes in the tee pipe, so they are cut away.)
cat > "$TMP/fail3.py" <<'PY'
raise ValueError("zz-pollution-probe")
PY
printf '%s\n' \
  "python3 \"$TMP/fail2.py\"" \
  "err pending" \
  "python3 \"$TMP/fail3.py\"" \
  'echo POLLUTE-DONE' \
  | bash --rcfile "$TMP/rc" -i >"$TMP/out4.txt" 2>&1
newid=$("$ERR" pending 2>/dev/null | grep 'zz-pollution-probe' | awk '{print $1}')
check "pollution probe recorded" test -n "$newid"
"$ERR" show "$newid" >"$TMP/shown.txt" 2>&1
check "signature unpolluted by err output" grep -qE '^signature: +ValueError: zz-pollution-probe$' "$TMP/shown.txt"
check "command kept after err pending ran" grep -q "command:.*python3 .*fail3.py" "$TMP/shown.txt"

# 11. Flag fidelity: a failing command's flags must survive verbatim into
#     errors.command (an old hook once recorded 'rmdir -f tmp' as
#     'rmdir tmp'). A missing-script python run exits 2 with a
#     recognizable "can't open file" error.
printf '%s\n' \
  "python3 -u \"$TMP/no_such_script_xyz.py\"" \
  'echo FLAG-DONE' \
  | bash --rcfile "$TMP/rc" -i >"$TMP/out5.txt" 2>&1
fid=$("$ERR" pending --all 2>/dev/null | grep "can't open file" | awk '{print $1}')
check "flag command recorded" test -n "$fid"
"$ERR" show "$fid" >"$TMP/showf.txt" 2>&1
check "flag preserved in command" grep -q "command:.*python3 -u .*no_such_script_xyz.py" "$TMP/showf.txt"

# 12. fish (and other shells) are rejected gracefully.
fish_out=$("$ERR" init fish 2>"$TMP/fish.err")
[ -z "$fish_out" ] && grep -q 'no shell hook' "$TMP/fish.err"
check "fish rejected gracefully" test $? -eq 0

if [ "$fails" -gt 0 ]; then
  echo "---"
  echo "$fails check(s) FAILED; session output:"
  cat "$TMP/out.txt"
  exit 1
fi
echo "all bash hook checks passed"
