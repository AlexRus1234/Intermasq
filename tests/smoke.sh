#!/usr/bin/env bash
# tests/smoke.sh — Intermasq smoke test suite.
#
# Runs ~50 HTTP checks against a running intermasq binary. Covers:
#   - auth (setup, login, JWT, X-API-Key, logout)
#   - static hosts CRUD + known bug regressions (A3 zero-MAC, A4 dash-MAC, A6 CSV count)
#   - DNS aliases CRUD + A2 duplicate-allowed regression
#   - config editor (create file, edit directive, raw PUT, delete)
#   - safety (backup, restore, history list/diff/restore)
#   - users CRUD
#   - audit log presence
#   - /metrics auth (4 methods) + A8 body-on-401 regression
#   - path traversal battery (A11)
#
# Failing tests are BY DESIGN for known bugs — see логи/predrel-v3-bugs-and-automation.md.
# A green run = bug fixes landed.
#
# Resilience: a fatal pre-condition failure (e.g. no JWT obtained) does NOT
# abort the script — dependent tests are SKIPped, the script continues, and
# all issues are summarised at the end.
#
# Usage:
#   export INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXX"
#   ./intermasq -port 18081 -conf-dir /tmp/conf -init-system=none -ci-mode=true &
#   BASE=http://localhost:18081 ./tests/smoke.sh

set -u

BASE="${BASE:-http://localhost:8081}"
SECRET="${INTERMASQ_SECRET:?INTERMASQ_SECRET must be set}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-pass1234}"
CONF_DIR="${CONF_DIR:-/tmp/intermasq-smoke-conf}"

PASS=0
FAIL=0
KNOWN_FAIL=0
SKIP=0
FATALS=()    # accumulated pre-condition failures (printed at the end)

# Colors (disabled if not a tty)
if [ -t 1 ]; then
    GREEN=$'\033[32m'; RED=$'\033[31m'; YELLOW=$'\033[33m'; CYAN=$'\033[36m'; BLUE=$'\033[34m'; RESET=$'\033[0m'
else
    GREEN=""; RED=""; YELLOW=""; CYAN=""; BLUE=""; RESET=""
fi

section() { printf "\n${CYAN}=== %s ===${RESET}\n" "$1"; }

# check(desc, expected_status, actual_status, [bug_id])
# Without bug_id:        pass if actual == expected
# With bug_id:           known-fail if actual != expected (still counted as known issue)
# Returns 0 on pass, 1 on any kind of fail (useful for branching).
check() {
    local desc="$1" exp="$2" got="$3" bug="${4:-}"
    if [ "$exp" = "$got" ]; then
        printf "  ${GREEN}✓${RESET} %s\n" "$desc"
        PASS=$((PASS + 1)); return 0
    elif [ -n "$bug" ]; then
        printf "  ${YELLOW}✗ KNOWN(%s)${RESET} %s (got %s, want %s)\n" "$bug" "$desc" "$got" "$exp"
        KNOWN_FAIL=$((KNOWN_FAIL + 1)); return 1
    else
        printf "  ${RED}✗${RESET} %s (got %s, want %s)\n" "$desc" "$got" "$exp"
        FAIL=$((FAIL + 1)); return 1
    fi
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

###############################################################################
# Pre-flight: clean state
###############################################################################

JWT=""

section "pre-flight"

rm -rf "$CONF_DIR"
mkdir -p "$CONF_DIR"
S=$(PGET "/api/status")
check "GET /api/status (no auth)" 200 "$S" || true
echo "  status body: $(body)"

# Read setup_required from body — NOTE: do NOT pipe PGET into jval,
# PGET returns HTTP code on stdout (body goes to /tmp/smoke.body file).
PGET "/api/status" >/dev/null
USERS=$(body | jval .setup_required)
echo "  setup_required=$USERS"

if [ "$USERS" = "true" ]; then
    S=$(PPOST "/api/setup" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
    check "POST /api/setup (create admin)" 200 "$S" || true
    JWT=$(body | jval .token)
else
    S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
    check "POST /api/login (existing admin)" 200 "$S" || true
    JWT=$(body | jval .token)
fi

if ! have_jwt; then
    fatal "no JWT obtained (setup/login failed) — most auth-required tests will be skipped"
fi
have_jwt && echo "  JWT: ${JWT:0:30}..."

###############################################################################
# Auth (requires JWT for some tests)
###############################################################################

section "auth"

S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"wrong\"}")
check "Login wrong password → 401" 401 "$S" || true

if have_jwt; then
    S=$(GET "$JWT" "/api/hosts")
    check "GET /api/hosts with valid JWT" 200 "$S" || true
else
    skip "GET /api/hosts with valid JWT"
fi

S=$(GET "invalid.jwt.token" "/api/hosts")
check "GET /api/hosts with garbage JWT → 401" 401 "$S" || true

S=$(KGET "$SECRET" "/api/hosts")
check "GET /api/hosts with X-API-Key" 200 "$S" || true

S=$(KGET "wrong-secret" "/api/hosts")
check "GET /api/hosts with wrong X-API-Key → 401" 401 "$S" || true

# Rate limit: 12 rapid bad logins from same IP (limit=10/min)
section "auth rate-limit"
RL_BLOCKED=0
RL_HIT=""
for i in $(seq 1 12); do
    S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"x\"}")
    if [ "$S" = "429" ]; then RL_BLOCKED=1; RL_HIT=$i; break; fi
done
if [ "$RL_BLOCKED" = "1" ]; then
    check "12 bad logins → 429 (rate limit triggered)" 429 "$S" || true
    echo "  blocked after $RL_HIT attempts"
else
    # Did not hit rate limit — could be that successful logins reset the counter
    # (which is A6-related behaviour). Mark as known-issue rather than hard fail.
    check "12 bad logins → 429 (rate limit triggered)" 429 "$S" || true
fi

###############################################################################
# Static hosts
###############################################################################

FILE="$CONF_DIR/10-static.conf"

if require_jwt "static hosts — happy path" 12; then
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:01\",\"ip\":\"10.0.0.11\",\"hostname\":\"test1\",\"file\":\"$FILE\"}")
    check "Add host (full record)" 200 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:01\",\"ip\":\"10.0.0.99\",\"hostname\":\"dupmac\",\"file\":\"$FILE\"}")
    check "Add duplicate MAC → 409" 409 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:02\",\"ip\":\"10.0.0.11\",\"hostname\":\"dupip\",\"file\":\"$FILE\"}")
    check "Add duplicate IP → 409" 409 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:03\",\"hostname\":\"noip\",\"file\":\"$FILE\"}")
    check "Add host without IP (optional)" 200 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:04\",\"ip\":\"10.0.0.14\",\"file\":\"$FILE\"}")
    check "Add host without hostname (optional)" 200 "$S" || true

    # Verify file content
    if [ -f "$FILE" ]; then
        LINES=$(grep -c "^dhcp-host=" "$FILE" || echo 0)
        check "File has 4 dhcp-host lines" 4 "$LINES" || true
    else
        check "File created" 4 0 || true
    fi

    # Tags
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:05\",\"ip\":\"10.0.0.15\",\"hostname\":\"tagged\",\"tags\":[\"set:iot\",\"tag:guest\"],\"file\":\"$FILE\"}")
    check "Add host with set:iot,tag:guest" 200 "$S" || true
    if [ -f "$FILE" ] && grep -q "set:iot,tag:guest" "$FILE"; then
        check "Tags written in file" 0 0 || true
    else
        check "Tags written in file" 0 1 || true
    fi

    # Invalid tag
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:06\",\"ip\":\"10.0.0.16\",\"hostname\":\"badtag\",\"tags\":[\"xyz:foo\"],\"file\":\"$FILE\"}")
    check "Add host with invalid tag → 400" 400 "$S" || true
fi

if require_jwt "static hosts — bug regressions" 3; then
    # A3: zero MAC should be rejected (currently accepted)
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"00:00:00:00:00:00\",\"ip\":\"10.0.0.99\",\"hostname\":\"zeromac\",\"file\":\"$FILE\"}")
    check "A3: zero MAC rejected → 400" 400 "$S" A3 || true

    # A3b: broadcast MAC should be rejected
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"ff:ff:ff:ff:ff:ff\",\"ip\":\"10.0.0.98\",\"hostname\":\"bcastmac\",\"file\":\"$FILE\"}")
    check "A3: broadcast MAC rejected → 400" 400 "$S" A3 || true

    # A4: dash separator should be normalized OR rejected (currently saved verbatim, breaks dnsmasq)
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa-bb-cc-dd-ee-07\",\"ip\":\"10.0.0.17\",\"hostname\":\"dashmac\",\"file\":\"$FILE\"}")
    check "A4: dash-MAC handled (rejected or normalized)" 400 "$S" A4 || true
fi

if require_jwt "static hosts — delete & list" 3; then
    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:01?file=$FILE")
    check "Delete host by MAC" 200 "$S" || true

    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:01?file=$FILE")
    check "Delete again → 404" 404 "$S" || true

    S=$(GET "$JWT" "/api/hosts")
    check "GET /api/hosts" 200 "$S" || true
    HOST_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  current host count: $HOST_COUNT"
fi

if require_jwt "static hosts — CSV import (A6)" 2; then
    CSV=$(mktemp)
    cat > "$CSV" <<EOF
mac,ip,hostname
aa:bb:cc:dd:ee:10,10.0.0.20,csv1
aa:bb:cc:dd:ee:11,10.0.0.21,csv2
aa:bb:cc:dd:ee:12,10.0.0.22,csv3
EOF
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@$CSV" -F "target_file=$FILE" "$BASE/api/hosts/csv")
    check "CSV import 3 hosts" 200 "$S" || true
    CSV_COUNT=$(body | jval .count)
    check "A6: CSV response has count=3" 3 "$CSV_COUNT" A6 || true
    rm -f "$CSV"
fi

if require_jwt "static hosts — bulk add (no count regression)" 2; then
    S=$(POST "$JWT" "/api/hosts/bulk" "{\"file\":\"$FILE\",\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:20\",\"ip\":\"10.0.0.30\",\"hostname\":\"bulk1\"},{\"mac\":\"aa:bb:cc:dd:ee:21\",\"ip\":\"10.0.0.31\",\"hostname\":\"bulk2\"}]}")
    check "Bulk add 2 hosts → 200" 200 "$S" || true
    BULK_COUNT=$(body | jval .count)
    check "Bulk JSON response has count field" 2 "$BULK_COUNT" A6 || true
fi

if require_jwt "static hosts — CSV export" 1; then
    S=$(GET "$JWT" "/api/hosts/csv")
    check "CSV export" 200 "$S" || true
    EXPORT_LINES=$(body | wc -l)
    echo "  csv export lines: $EXPORT_LINES"
fi

###############################################################################
# DNS Aliases
###############################################################################

ALIAS_FILE="$CONF_DIR/20-aliases.conf"

if require_jwt "DNS aliases — happy path" 6; then
    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"nas.local\",\"target\":\"10.0.0.5\",\"file\":\"$ALIAS_FILE\"}")
    check "Add A record" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"CNAME\",\"domain\":\"www.local\",\"target\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Add CNAME" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"PTR\",\"domain\":\"5.0.0.10.in-addr.arpa\",\"target\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Add PTR" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"TXT\",\"domain\":\"_dmarc.local\",\"target\":\"v=DMARC1;p=reject\",\"file\":\"$ALIAS_FILE\"}")
    check "Add TXT" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"bad.local\",\"target\":\"not-an-ip\",\"file\":\"$ALIAS_FILE\"}")
    check "A with non-IP target → 400" 400 "$S" || true
fi

if require_jwt "DNS aliases — A2 regression" 2; then
    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"nas.local\",\"target\":\"10.0.0.99\",\"file\":\"$ALIAS_FILE\"}")
    check "A2: duplicate A same file → 409" 409 "$S" A2 || true

    if [ -f "$ALIAS_FILE" ]; then
        DUP_COUNT=$(grep -c "^address=/nas\.local/" "$ALIAS_FILE" || echo 0)
        check "A2: file has exactly 1 nas.local A record" 1 "$DUP_COUNT" A2 || true
    else
        skip "A2: file has exactly 1 nas.local A record (file missing)"
    fi
fi

if require_jwt "DNS aliases — delete" 3; then
    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"A\",\"domain\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete A record" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"A\",\"domain\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete again → 404" 404 "$S" || true

    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"PTR\",\"domain\":\"5.0.0.10.in-addr.arpa\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete PTR rejected (UI only supports A/CNAME)" 400 "$S" || true
fi

###############################################################################
# Config files
###############################################################################

if require_jwt "config files" 10; then
    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"30-test.conf\",\"template\":\"empty\"}")
    check "Create 30-test.conf from empty template" 200 "$S" || true

    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"40-dhcp.conf\",\"template\":\"basic-dhcp\"}")
    check "Create 40-dhcp.conf from basic-dhcp template" 200 "$S" || true

    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"foo.txt\",\"template\":\"empty\"}")
    check "Reject non-.conf name → 400" 400 "$S" || true

    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"../../../tmp/evil.conf\",\"template\":\"empty\"}")
    check "Reject path-traversal name → 400" 400 "$S" || true

    S=$(GET "$JWT" "/api/config/templates")
    check "List config templates" 200 "$S" || true

    S=$(GET "$JWT" "/api/config")
    check "GET /api/config snapshot" 200 "$S" || true
    FILE_COUNT=$(body | jq '.files | length' 2>/dev/null || echo "?")
    echo "  config files in snapshot: $FILE_COUNT"

    S=$(GET "$JWT" "/api/files/30-test.conf")
    check "GET raw file" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"# test\ndomain-needed\nbogus-priv\n"}' "$BASE/api/files/30-test.conf")
    check "PUT raw file with valid content" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"# broken\ndhcp-host=\n"}' "$BASE/api/files/30-test.conf")
    check "PUT with invalid dnsmasq syntax → 400" 400 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"file":"'"$CONF_DIR"'/30-test.conf"}' "$BASE/api/config/file")
    check "DELETE config file" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"file":"'"$CONF_DIR"'/30-test.conf"}' "$BASE/api/config/file")
    check "DELETE missing file → 404" 404 "$S" || true
fi

###############################################################################
# Safety: backup, restore, history
###############################################################################

if require_jwt "safety — backup / history" 3; then
    S=$(GET "$JWT" "/api/backup")
    check "GET /api/backup (ZIP download)" 200 "$S" || true
    ZIP_SIZE=$(wc -c < /tmp/smoke.body)
    echo "  backup zip size: $ZIP_SIZE bytes"

    S=$(GET "$JWT" "/api/history?file=$FILE")
    check "GET /api/history for static file" 200 "$S" || true
    HIST_COUNT=$(body | jq '.versions | length' 2>/dev/null || echo "?")
    echo "  history versions: $HIST_COUNT"

    # Rollback test — only attempt if history exists
    if [ "${HIST_COUNT:-0}" -gt 0 ]; then
        S=$(POST "$JWT" "/rollback" "{\"file\":\"$FILE\"}")
        check "POST /api/rollback" 200 "$S" || true
    else
        skip "POST /api/rollback (no history yet)"
    fi
fi

###############################################################################
# Users
###############################################################################

if require_jwt "users" 9; then
    S=$(GET "$JWT" "/api/users")
    check "GET /api/users" 200 "$S" || true

    S=$(POST "$JWT" "/api/users" "{\"username\":\"alice\",\"password\":\"alicepass\"}")
    check "Create user alice" 200 "$S" || true

    S=$(POST "$JWT" "/api/users" "{\"username\":\"alice\",\"password\":\"alicepass\"}")
    check "Create alice again → 409" 409 "$S" || true

    S=$(POST "$JWT" "/api/users" "{\"username\":\"$(printf 'x%.0s' {1..70})\",\"password\":\"y\"}")
    check "Create too-long username → 400" 400 "$S" || true

    S=$(DELETE "$JWT" "/api/users/alice")
    check "Delete user alice" 200 "$S" || true

    S=$(DELETE "$JWT" "/api/users/alice")
    check "Delete missing user → 404" 404 "$S" || true

    S=$(DELETE "$JWT" "/api/users/$ADMIN_USER")
    check "Cannot delete self → 400" 400 "$S" || true

    # Change own password (correct old)
    S=$(POST "$JWT" "/api/users/password" "{\"old_password\":\"$ADMIN_PASS\",\"new_password\":\"newpass\"}")
    check "Change own password (correct old)" 200 "$S" || true

    # Change back so we can keep testing
    S=$(POST "$JWT" "/api/users/password" "{\"old_password\":\"newpass\",\"new_password\":\"$ADMIN_PASS\"}")
    check "Change password back" 200 "$S" || true

    # Change with wrong old
    S=$(POST "$JWT" "/api/users/password" "{\"old_password\":\"wrong\",\"new_password\":\"x\"}")
    check "Change password wrong old → 401" 401 "$S" || true
fi

###############################################################################
# Audit log
###############################################################################

if require_jwt "audit" 2; then
    S=$(GET "$JWT" "/api/audit")
    check "GET /api/audit" 200 "$S" || true
    AUDIT_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  audit entries: $AUDIT_COUNT"
    if [ "${AUDIT_COUNT:-0}" -gt 0 ]; then
        check "Audit log non-empty" 1 1 || true
    else
        check "Audit log non-empty" 1 0 || true
    fi
fi

###############################################################################
# Metrics (some tests don't need JWT)
###############################################################################

section "metrics"

# A8: /metrics without auth returns 401 but with EMPTY body (cosmetic issue).
S=$(PGET "/metrics")
check "GET /metrics without auth → 401" 401 "$S" || true
METRICS_NOAUTH_BODY_SIZE=$(wc -c < /tmp/smoke.body)
echo "  body bytes on 401: $METRICS_NOAUTH_BODY_SIZE"
if [ "$METRICS_NOAUTH_BODY_SIZE" -gt 2 ]; then
    check "A8: 401 has body (currently empty)" 1 1 A8 || true
else
    check "A8: 401 has body (currently empty)" 1 0 A8 || true
fi

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer ${JWT:-invalid}" "$BASE/metrics")
check "GET /metrics with Bearer JWT" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "X-API-Key: $SECRET" "$BASE/metrics")
check "GET /metrics with X-API-Key" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$BASE/metrics?token=$SECRET")
check "GET /metrics with ?token=" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$BASE/metrics?token=wrong")
check "GET /metrics with wrong ?token= → 401" 401 "$S" || true

# Spot-check metric names
curl -s -H "X-API-Key: $SECRET" "$BASE/metrics" > /tmp/smoke.metrics
for m in intermasq_hosts_total intermasq_reloads_total intermasq_dnsmasq_active intermasq_uptime_seconds; do
    if grep -q "^$m " /tmp/smoke.metrics; then
        check "metric $m present" 0 0 || true
    else
        check "metric $m present" 0 1 || true
    fi
done

###############################################################################
# Path traversal battery (A11)
###############################################################################

if require_jwt "path traversal (A11)" 9; then
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:f1\",\"file\":\"/etc/passwd\"}")
    check "Hosts file=/etc/passwd rejected" 400 "$S" || true

    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:f1?file=/etc/passwd")
    check "DELETE host file=/etc/passwd rejected" 400 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"evil.test\",\"target\":\"10.0.0.1\",\"file\":\"../../../tmp/x.conf\"}")
    check "Aliases file traversal rejected" 403 "$S" || true

    S=$(GET "$JWT" "/api/files/..%2F..%2Fetc%2Fpasswd")
    check "GET raw file traversal rejected" 403 "$S" || true

    S=$(curl -s -o /dev/null -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"x"}' "$BASE/api/files/passwd")
    check "PUT raw file non-.conf rejected" 403 "$S" || true

    S=$(POST "$JWT" "/api/history/restore" "{\"file\":\"/etc/hosts\",\"version\":\"20240101-000000\"}")
    check "History restore file=/etc/hosts rejected" 400 "$S" || true

    S=$(GET "$JWT" "/api/history?file=/etc/shadow")
    check "GET history file=/etc/shadow rejected" 400 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:f2\",\"hostname\":\"a\\ndhcp-host=evil\",\"file\":\"$FILE\"}")
    check "Hostname with newline rejected" 400 "$S" || true
fi

###############################################################################
# Logout
###############################################################################

if have_jwt; then
    section "logout"
    S=$(POST "$JWT" "/api/logout" "{}")
    check "POST /api/logout" 200 "$S" || true

    S=$(GET "$JWT" "/api/hosts")
    check "Old JWT after logout → 401 (blacklist works)" 401 "$S" || true
else
    skip "logout section (no JWT)"
fi

###############################################################################
# Summary
###############################################################################

section "SUMMARY"
TOTAL=$((PASS + FAIL + KNOWN_FAIL + SKIP))
echo
printf "  ${GREEN}Pass:        %d${RESET} / %d\n" "$PASS" "$TOTAL"
printf "  ${RED}Fail:        %d${RESET} / %d  (unexpected — investigate)\n" "$FAIL" "$TOTAL"
printf "  ${YELLOW}Known-fail:  %d${RESET} / %d  (bugs A2/A3/A4/A6/A8/A11 — to be fixed)\n" "$KNOWN_FAIL" "$TOTAL"
printf "  ${BLUE}Skipped:     %d${RESET} / %d  (pre-condition failed)\n" "$SKIP" "$TOTAL"
echo

if [ ${#FATALS[@]} -gt 0 ]; then
    printf "${RED}FATALS (pre-condition failures):${RESET}\n"
    for f in "${FATALS[@]}"; do
        printf "  • %s\n" "$f"
    done
    echo
fi

if [ "$FAIL" -gt 0 ]; then
    printf "${RED}UNEXPECTED FAILURES — investigate.${RESET}\n"
    exit 1
fi
if [ ${#FATALS[@]} -gt 0 ]; then
    printf "${RED}Pipeline RED due to pre-condition failures.${RESET}\n"
    exit 1
fi
if [ "$KNOWN_FAIL" -gt 0 ]; then
    printf "${YELLOW}All failures are known bugs (regression tests). Pipeline green.${RESET}\n"
    exit 0
fi
printf "${GREEN}CLEAN PASS.${RESET}\n"
exit 0
