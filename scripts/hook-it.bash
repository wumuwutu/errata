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
  'echo hello-world' \
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
check "hit hint shown" grep -q '遇到过此错误' "$TMP/out.txt"
# 5. Session actually completed (hook did not hang the shell).
check "session completed" grep -q 'SESSION-DONE' "$TMP/out.txt"
# 6. The ignore blacklist applies on the hook path too.
"$ERR" ignore add --command python3 >/dev/null 2>&1
printf '%s\n' \
  "python3 \"$TMP/fail.py\"" \
  'echo IGNORE-CHECK-DONE' \
  | bash --rcfile "$TMP/rc" -i >"$TMP/out2.txt" 2>&1
"$ERR" show 1 >"$TMP/show1b.txt" 2>&1
check "ignore respected on hook path" grep -q '2 times' "$TMP/show1b.txt"
"$ERR" ignore remove --command python3 >/dev/null 2>&1
# 7. fish (and other shells) are rejected gracefully.
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
