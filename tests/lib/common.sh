# tests/lib/common.sh — output helpers + check()/skip()/fatal() primitives.
#
# Depends on lib/state.sh (PASS/FAIL/KNOWN_FAIL/SKIP counters, FATALS,
# KNOWN_BUGS map). Must be sourced AFTER state.sh.

# Colors (disabled if not a tty)
if [ -t 1 ]; then
    GREEN=$'\033[32m'; RED=$'\033[31m'; YELLOW=$'\033[33m'; CYAN=$'\033[36m'; BLUE=$'\033[34m'; RESET=$'\033[0m'
else
    GREEN=""; RED=""; YELLOW=""; CYAN=""; BLUE=""; RESET=""
fi

section() { printf "\n${CYAN}=== %s ===${RESET}\n" "$1"; }

# check(desc, expected_status, actual_status, [bug_id])
#
# Without bug_id:                 pass if actual == expected, else FAIL (red).
# With bug_id IN known-bugs.txt:  KNOWN-fail (yellow) if mismatch — pipeline
#                                 stays green, but the bug is documented.
# With bug_id NOT in known-bugs.txt and mismatch:
#                                 loud FAIL (red) prompting test update —
#                                 the bug was fixed but the test still
#                                 asserts the old (broken) behaviour, OR
#                                 the bug ID needs to be added to
#                                 known-bugs.txt because it's a new issue.
# Returns 0 on pass, 1 on any kind of fail.
check() {
    local desc="$1" exp="$2" got="$3" bug="${4:-}"
    if [ "$exp" = "$got" ]; then
        printf "  ${GREEN}✓${RESET} %s\n" "$desc"
        PASS=$((PASS + 1)); return 0
    fi
    if [ -n "$bug" ]; then
        if [ "${KNOWN_BUGS[$bug]:-}" = "1" ]; then
            printf "  ${YELLOW}✗ KNOWN(%s)${RESET} %s (got %s, want %s)\n" "$bug" "$desc" "$got" "$exp"
            KNOWN_FAIL=$((KNOWN_FAIL + 1)); return 1
        else
            printf "  ${RED}✗ FAIL(%s)${RESET} %s (got %s, want %s)\n" "$bug" "$desc" "$got" "$exp"
            printf "      ${RED}Bug %s not in known-bugs.txt${RESET} — either fix this test (bug already resolved)\n" "$bug"
            printf "      or add %s to tests/known-bugs.txt (new bug found).\n" "$bug"
            FAIL=$((FAIL + 1)); return 1
        fi
    fi
    printf "  ${RED}✗${RESET} %s (got %s, want %s)\n" "$desc" "$got" "$exp"
    FAIL=$((FAIL + 1)); return 1
}

# skip(desc): mark a test as skipped (pre-condition failed).
skip() {
    printf "  ${BLUE}-${RESET} %s (skipped)\n" "$1"
    SKIP=$((SKIP + 1))
}

# fatal(desc): record a pre-condition failure. Does NOT exit — caller continues.
fatal() {
    local msg="$1"
    FATALS+=("$msg")
    printf "  ${RED}‼ FATAL${RESET}: %s — continuing with dependent tests skipped\n" "$msg"
}
