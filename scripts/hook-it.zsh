#!/usr/bin/env bash
# Integration test for the zsh shell hook (driver is bash; the session
# under test runs zsh).
# Usage: hook-it.zsh <path-to-err-binary>
set -u

ERR="$1"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/dejavu-hooktest-zsh.XXXXXX")
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
raise TypeError("unsupported operand type(s) for +: 'int' and 'str'")
PY

# Isolated ZDOTDIR: .zshrc contains only the hook (with err on PATH).
mkdir -p "$TMP/zdot"
cat > "$TMP/zdot/.zshrc" <<EOF
export XDG_DATA_HOME='$XDG_DATA_HOME'
export XDG_CONFIG_HOME='$XDG_CONFIG_HOME'
export PATH='$(dirname "$ERR")':"\$PATH"
eval "\$('$ERR' init zsh)"
EOF

printf '%s\n' \
  'false; echo "EC=$?"' \
  "python3 \"$TMP/fail.py\"" \
  'true' \
  "python3 -c 'print(1)'" \
  "python3 \"$TMP/fail.py\"" \
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
# 6. Success detection: same-program success nudges once; the unrelated
#    'true' and the echo before SESSION-DONE must stay quiet.
check "success hint shown" grep -q 'looks fixed' "$TMP/out.txt"
hint_count=$(grep -c 'looks fixed' "$TMP/out.txt")
check "success hint not repeated" test "$hint_count" -eq 1
# 6b. Command attribution: the recorded command is the one that failed.
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

if [ "$fails" -gt 0 ]; then
  echo "---"
  echo "$fails check(s) FAILED; session output:"
  cat "$TMP/out.txt"
  exit 1
fi
echo "all zsh hook checks passed"
