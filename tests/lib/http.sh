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
# as -1 (always fails the >= min check). For plain-text/CSV responses use an
# inline `check` with `wc -l` instead.
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
