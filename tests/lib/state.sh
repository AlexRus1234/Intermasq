# tests/lib/state.sh — global run state, counters, config, known-bugs loader.
#
# Sourced by tests/smoke.sh BEFORE any other lib: sets up config vars
# (BASE, SECRET, ...), global counters (PASS/FAIL/KNOWN_FAIL/SKIP), the
# FATALS array, the KNOWN_BUGS map (loaded by init_state) and the shared
# JWT slot. print_summary() renders the final report and returns the
# exit code for the orchestrator to propagate.

BASE="${BASE:-http://localhost:8081}"
SECRET="${INTERMASQ_SECRET:?INTERMASQ_SECRET must be set}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-pass1234}"
# MUST match the -conf-dir flag passed to the binary. Default /tmp/conf
# matches .forgejo/workflows/build.yml. Override via env if running locally
# against a different path.
CONF_DIR="${CONF_DIR:-/tmp/conf}"
# Path to the known-bugs list. Override via env if needed.
# $0 here is the orchestrator (tests/smoke.sh), so dirname resolves to tests/.
KNOWN_BUGS_FILE="${KNOWN_BUGS_FILE:-$(dirname "$0")/known-bugs.txt}"

PASS=0
FAIL=0
KNOWN_FAIL=0
SKIP=0
FATALS=()    # accumulated pre-condition failures (printed at the end)

# Shared mutable state across suites.
JWT=""

# Load known-bugs list into an associative array.
# Each bug ID listed here means "test failures tagged with this ID are
# expected". Remove the ID when the bug is fixed — the corresponding
# check will then turn into a loud FAIL prompting you to update the test.
declare -A KNOWN_BUGS=()
KNOWN_BUGS_LIST=""

init_state() {
    if [ -f "$KNOWN_BUGS_FILE" ]; then
        while IFS= read -r _line; do
            _line="${_line%%#*}"                # strip comments
            _line="$(echo "$_line" | xargs)"    # trim whitespace
            [ -z "$_line" ] && continue
            _id="${_line%%[[:space:]]*}"        # first token = bug ID
            [ -n "$_id" ] && KNOWN_BUGS["$_id"]=1
        done < "$KNOWN_BUGS_FILE"
    fi
    KNOWN_BUGS_LIST="$(echo "${!KNOWN_BUGS[@]}" | tr ' ' '\n' | sort | tr '\n' ' ' | sed 's/ $//')"
}

print_summary() {
    local total
    total=$((PASS + FAIL + KNOWN_FAIL + SKIP))

    printf "\n${CYAN}=== SUMMARY ===${RESET}\n"
    echo
    printf "  ${GREEN}Pass:        %d${RESET} / %d\n" "$PASS" "$total"
    printf "  ${RED}Fail:        %d${RESET} / %d  (unexpected — investigate)\n" "$FAIL" "$total"
    printf "  ${YELLOW}Known-fail:  %d${RESET} / %d  (bugs: %s)\n" "$KNOWN_FAIL" "$total" "${KNOWN_BUGS_LIST:-(none)}"
    printf "  ${BLUE}Skipped:     %d${RESET} / %d  (pre-condition failed)\n" "$SKIP" "$total"
    echo

    if [ ${#FATALS[@]} -gt 0 ]; then
        printf "${RED}FATALS (pre-condition failures):${RESET}\n"
        for _f in "${FATALS[@]}"; do
            printf "  • %s\n" "$_f"
        done
        echo
    fi

    if [ "$FAIL" -gt 0 ]; then
        printf "${RED}UNEXPECTED FAILURES — investigate.${RESET}\n"
        return 1
    fi
    if [ ${#FATALS[@]} -gt 0 ]; then
        printf "${RED}Pipeline RED due to pre-condition failures.${RESET}\n"
        return 1
    fi
    if [ "$KNOWN_FAIL" -gt 0 ]; then
        printf "${YELLOW}All failures are known bugs (regression tests). Pipeline green.${RESET}\n"
        return 0
    fi
    printf "${GREEN}CLEAN PASS.${RESET}\n"
    return 0
}
