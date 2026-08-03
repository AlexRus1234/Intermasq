# tests/lib/http.sh — curl wrappers + JSON helpers.
#
# All wrappers write the response body to /tmp/smoke.body and print the
# HTTP status code on stdout. body() reads the last written body.
# Depends on lib/state.sh (BASE, SECRET).

# jq-like helper: extract JSON field from stdin (requires jq)
jval() { jq -r "$1" 2>/dev/null; }

# HTTP request helpers (status code on stdout, body to /tmp/smoke.body)
GET()    { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $1" "$BASE$2"; }
POST()   { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $1" -H "Content-Type: application/json" -X POST -d "$3" "$BASE$2"; }
DELETE() { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $1" -X DELETE "$BASE$2"; }
PGET()   { curl -s -o /tmp/smoke.body -w "%{http_code}" "$BASE$1"; }                # no-auth GET
PPOST()  { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Content-Type: application/json" -X POST -d "$2" "$BASE$1"; }  # no-auth POST
KGET()   { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "X-API-Key: $1" "$BASE$2"; }  # api-key GET

body() { cat /tmp/smoke.body; }

# check_length(desc, min, [jq_expr]): assert the last response body is JSON
# whose `jq <expr>` is an integer >= <min>. <jq_expr> defaults to 'length'
# (top-level array/object length); pass e.g. '.files | length' for nested
# arrays. P2.1 guard: catches a handler silently returning [] / {} with 200
# when it should have data. Reads /tmp/smoke.body via body(). Bumps the
# PASS/FAIL counters from state.sh. Non-JSON / non-numeric jq output counts
# as -1 (always fails the >= min check). For plain-text/CSV responses use
# check_lines (wc -l based) instead — `check` itself is an EXACT equality
# test, so it is wrong for ">=" line-count guards.
check_length() {
    local desc="$1" min="$2" expr="${3:-length}"
    local count
    count=$(body | jq "$expr" 2>/dev/null)
    case "$count" in
        ''|*[!0-9-]*) count=-1 ;;
    esac
    if [ "$count" -ge "$min" ] 2>/dev/null; then
        printf "  ${GREEN}✓${RESET} %s (%s=%s)\n" "$desc" "$expr" "$count"
        PASS=$((PASS + 1))
    else
        printf "  ${RED}✗${RESET} %s (%s=%s, want >= %s)\n" "$desc" "$expr" "$count" "$min"
        FAIL=$((FAIL + 1))
    fi
}

# check_lines(desc, min): assert the last response body has at least <min>
# newline-terminated lines (wc -l). Same P2.1 purpose as check_length but
# for plain-text/CSV bodies where jq does not apply. Reads /tmp/smoke.body
# via body(). Note: wc -l counts newlines, so a body without a trailing
# newline on its last line undercounts by 1 — the CI CSV exports always
# terminate with \n, so the threshold stays correct.
check_lines() {
    local desc="$1" min="$2"
    local count
    count=$(body | wc -l | tr -d ' ')
    case "$count" in
        ''|*[!0-9-]*) count=-1 ;;
    esac
    if [ "$count" -ge "$min" ] 2>/dev/null; then
        printf "  ${GREEN}✓${RESET} %s (lines=%s)\n" "$desc" "$count"
        PASS=$((PASS + 1))
    else
        printf "  ${RED}✗${RESET} %s (lines=%s, want >= %s)\n" "$desc" "$count" "$min"
        FAIL=$((FAIL + 1))
    fi
}
