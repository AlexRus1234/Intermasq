#!/usr/bin/env bash
# tests/smoke.sh — Intermasq smoke test suite entrypoint / orchestrator.
#
# Runs ~80 HTTP checks against a running intermasq binary. The actual
# checks live in tests/suites/NN-*.sh (one file per area: auth, hosts,
# aliases, config, safety, users, audit, metrics, path-traversal,
# logout). Shared helpers live in tests/lib/.
#
# Coverage:
#   - auth (setup, login, JWT, X-API-Key, logout)
#   - static hosts CRUD + known bug regressions (A3 zero-MAC, A4 dash-MAC, A6 CSV count)
#   - DNS aliases CRUD + A2 duplicate-allowed regression
#   - config editor (create file, edit directive, raw PUT, delete)
#   - safety (backup, restore, history list/diff/restore)
#   - users CRUD
#   - audit log presence
#   - /metrics auth (4 methods) + A8 body-on-401 regression
#   - path traversal battery (A11)
#   - apply-template / leases-to-static / restart-self / reload / SSE events (P3.8)
#
# Failing tests are BY DESIGN for known bugs — see tests/known-bugs.txt
# and tests/bugreport/bugs.md. A green run = bug fixes landed.
#
# Resilience: a fatal pre-condition failure (e.g. no JWT obtained) does NOT
# abort the script — dependent suites are SKIPped, the run continues, and
# all issues are summarised at the end.
#
# Usage:
#   export INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXX"
#   ./intermasq -port 18081 -conf-dir /tmp/conf -init-system=none -ci-mode=true &
#   BASE=http://localhost:18081 ./tests/smoke.sh

set -u

TESTS_DIR="$(cd "$(dirname "$0")" && pwd)"

# Order matters: state first (sets up counters, KNOWN_BUGS, config vars),
# then common (output helpers using colours), then http + auth.
source "$TESTS_DIR/lib/state.sh"
source "$TESTS_DIR/lib/common.sh"
source "$TESTS_DIR/lib/http.sh"
source "$TESTS_DIR/lib/auth.sh"

init_state

# Run each suite in lexical order. The NN- prefix guarantees the intended
# sequence (preflight → auth → hosts → aliases → config → … → logout).
# Each suite increments the global PASS/FAIL/KNOWN_FAIL/SKIP counters.
for _suite in "$TESTS_DIR/suites"/[0-9]*.sh; do
    [ -f "$_suite" ] || continue
    source "$_suite"
done

# Translate accumulated state into an exit code for the pipeline:
#   0 — clean pass OR all failures are KNOWN bugs (pipeline green)
#   1 — any unexpected FAIL or pre-condition FATAL
print_summary
exit $?
