# tests/lib/auth.sh — JWT lifecycle helpers.
#
# have_jwt(): is there a usable JWT in the current run?
# require_jwt(section, count): if no JWT, skip the whole section with
# the given test count; otherwise print the section header.
# Depends on lib/state.sh (JWT, SKIP) and lib/common.sh (section).

# have_jwt: returns 0 if JWT is set and non-null, 1 otherwise.
have_jwt() { [ -n "${JWT:-}" ] && [ "${JWT:-}" != "null" ]; }

# require_jwt section_name approx_test_count: if no JWT, skip the whole section.
require_jwt() {
    local section="$1" count="$2"
    if ! have_jwt; then
        printf "\n${CYAN}=== %s ===${RESET}\n  ${BLUE}-- skipped${RESET} (no JWT: %s tests)\n" "$section" "$count"
        SKIP=$((SKIP + count))
        return 1
    fi
    section "$section"
    return 0
}
