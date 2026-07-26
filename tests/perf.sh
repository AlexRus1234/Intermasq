#!/usr/bin/env bash
# tests/perf.sh — opt-in performance/stress scenarios (Gap 5).
#
# DELIBERATELY SEPARATE from smoke.sh. Timing/memory thresholds flap on shared
# runners, so perf must NOT gate the main pipeline: CI only runs this when the
# run_perf_tests input is set. Hard failures are reserved for clear functional
# breaks (server dies, every SSE client drops, >5% 5xx); slow laps and modest
# RSS growth are WARNINGS that print metrics without failing the step.
#
# Like smoke.sh, this script assumes a running intermasq binary (the CI step
# starts it, waits for /api/status, then execs perf.sh). The CI step exports
# SERVER_PID so RSS can be sampled from /proc/<pid>/status; if absent, RSS
# tracking is skipped.
#
# Usage:
#   export INTERMASQ_SECRET="..."
#   ./intermasq -port 18082 -conf-dir /tmp/perf-conf -init-system=none -ci-mode=true &
#   export SERVER_PID=$!
#   BASE=http://localhost:18082 CONF_DIR=/tmp/perf-conf ./tests/perf.sh

set -u

TESTS_DIR="$(cd "$(dirname "$0")" && pwd)"

# Reuse the smoke helpers for config (BASE/SECRET/CONF_DIR), colours,
# section(), JWT plumbing and the HTTP wrappers. Perf does NOT reuse check()
# — its pass/fail semantics differ (see ok/warn/hard below).
source "$TESTS_DIR/lib/state.sh"
source "$TESTS_DIR/lib/common.sh"
source "$TESTS_DIR/lib/http.sh"
source "$TESTS_DIR/lib/auth.sh"
init_state

# --- Soft thresholds (env-overridable) --------------------------------------
# Defaults are deliberately CI-friendly (Fedora container, modest cores). Bump
# them via env to match aspirational targets (SSE_CLIENTS=50,
# SSE_SECONDS=60, CRUD_CYCLES=1000) on a dedicated runner.
SEED_HOSTS="${SEED_HOSTS:-200}"             # hosts written by gen-hosts.sh for read-load
READ_TOTAL="${READ_TOTAL:-200}"             # total GET /api/hosts requests
READ_CONCURRENCY="${READ_CONCURRENCY:-20}"
READ_RPS_FLOOR="${READ_RPS_FLOOR:-25}"      # min req/s before a WARNING
READ_5XX_CEIL_PCT="${READ_5XX_CEIL_PCT:-1}" # max % non-2xx before HARD fail
RELOAD_CONCURRENCY="${RELOAD_CONCURRENCY:-10}"
CRUD_CYCLES="${CRUD_CYCLES:-200}"
RSS_LEAK_MB="${RSS_LEAK_MB:-60}"            # max RSS growth before a WARNING
RSS_LEAK_HARD_MB="${RSS_LEAK_HARD_MB:-200}" # max RSS growth before HARD fail
SSE_CLIENTS="${SSE_CLIENTS:-20}"
SSE_SECONDS="${SSE_SECONDS:-15}"
SSE_DROP_HARD_PCT="${SSE_DROP_HARD_PCT:-50}" # >this % dropped => HARD fail

# --- Perf-specific result helpers -------------------------------------------
HARD_FAIL=0
ok()   { printf "  ${GREEN}✓${RESET} %s\n" "$1"; }
warn() { printf "  ${YELLOW}⚠${RESET} %s\n" "$1"; }
hard() { printf "  ${RED}✗${RESET} %s\n" "$1"; HARD_FAIL=$((HARD_FAIL + 1)); }

# RSS in KB straight from /proc — avoids any dependency on procps-ng (ps),
# which the minimal fedora:44 image may not ship. kill/wait are bash builtins.
rss_kb_of() { awk '/^VmRSS:/ {print $2}' "/proc/$1/status" 2>/dev/null; }

# --- Pre-flight: server alive + JWT -----------------------------------------
section "perf pre-flight"

if ! code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/status" 2>/dev/null) || [ "$code" != "200" ]; then
    printf "  ${RED}‼ FATAL${RESET}: server not reachable at %s (status=%s)\n" "$BASE" "${code:-none}"
    exit 1
fi
ok "server reachable at $BASE"

PGET "/api/status" >/dev/null
if [ "$(body | jval .setup_required)" = "true" ]; then
    S=$(PPOST "/api/setup" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
else
    S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
fi
JWT=$(body | jval .token)
if have_jwt; then
    ok "obtained JWT"
else
    hard "no JWT (setup/login returned $S)"
    printf "\n${RED}perf aborted (no auth).${RESET}\n"; exit 1
fi
rm -rf "$CONF_DIR"; mkdir -p "$CONF_DIR"

# ============================================================================
# Scenario 1 — read throughput against a large seeded dataset
# ============================================================================
section "read load — GET /api/hosts x$READ_TOTAL (concurrency $READ_CONCURRENCY)"

"$TESTS_DIR/fixtures/gen-hosts.sh" "$CONF_DIR/seed.conf" "$SEED_HOSTS" >/dev/null

# Sanity: the panel must see the seeded hosts.
S=$(GET "$JWT" "/api/hosts")
HOST_COUNT=$(body | jq 'length' 2>/dev/null || echo 0)
echo "  parsed host count: $HOST_COUNT (seeded $SEED_HOSTS)"
if [ "${HOST_COUNT:-0}" -ge "$SEED_HOSTS" ]; then ok "seeded dataset visible"; else hard "seeded dataset not fully parsed"; fi

rm -f /tmp/perf.read.codes
start=$(date +%s.%N)
# xargs runs curl directly (no sh -c): $JWT/$BASE expand in this shell when
# the argv is built. tr + -0 feeds null-delimited input so xargs does NO quote
# processing — kills the spurious "unmatched single quote" warning some xargs
# builds emit, with no downside for plain-integer input.
seq 1 "$READ_TOTAL" | tr '\n' '\0' | xargs -0 -P "$READ_CONCURRENCY" -I{} \
    curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $JWT" "$BASE/api/hosts" \
    > /tmp/perf.read.codes
end=$(date +%s.%N)

elapsed=$(awk "BEGIN{printf \"%.3f\", $end - $start}")
rps=$(awk "BEGIN{printf \"%.1f\", $READ_TOTAL / ($end - $start)}")
total_ok=$(grep -c '^2' /tmp/perf.read.codes || true)
total_bad=$(awk 'BEGIN{c=0} $0 !~ /^2/ {c++} END{print c+0}' /tmp/perf.read.codes)
bad_pct=$(awk "BEGIN{printf \"%.2f\", ($total_bad / $READ_TOTAL) * 100}")
echo "  $READ_TOTAL reqs in ${elapsed}s → ${rps} req/s | 2xx=$total_ok non-2xx=$total_bad (${bad_pct}%)"

if [ "$total_ok" -eq "$READ_TOTAL" ]; then ok "all reads 2xx"; else hard "$total_bad non-2xx responses (ceil ${READ_5XX_CEIL_PCT}%)"; fi
awk "BEGIN{exit !($bad_pct < $READ_5XX_CEIL_PCT)}" && ok "non-2xx within ${READ_5XX_CEIL_PCT}%" || hard "non-2xx ${bad_pct}% exceeds ${READ_5XX_CEIL_PCT}%"
awk "BEGIN{exit !($rps >= $READ_RPS_FLOOR)}" && ok "throughput ≥ ${READ_RPS_FLOOR} req/s" || warn "throughput ${rps} req/s below floor ${READ_RPS_FLOOR} (env-dependent, not fatal)"

# ============================================================================
# Scenario 2 — reload storm (N concurrent POST /api/reload)
# ============================================================================
# reloadDnsmasq() spawns `dnsmasq --test` then asks the init caller to restart.
# Under -init-system=none the restart is a no-op, so this exercises concurrent
# external-process spawning + the reload counter without touching dnsmasq. The
# invariant: no 5xx, all responses agree on one status code, server still up.
section "reload storm — POST /api/reload x$RELOAD_CONCURRENCY (concurrent)"

rm -f /tmp/perf.reload.codes
seq 1 "$RELOAD_CONCURRENCY" | tr '\n' '\0' | xargs -0 -P "$RELOAD_CONCURRENCY" -I{} \
    curl -s -o /dev/null -w "%{http_code}\n" -X POST -H "Authorization: Bearer $JWT" "$BASE/api/reload" \
    > /tmp/perf.reload.codes

distinct=$(sort -u /tmp/perf.reload.codes | wc -l)
reload_5xx=$(grep -c '^5' /tmp/perf.reload.codes || true)
echo "  status codes observed: $(sort /tmp/perf.reload.codes | uniq -c | tr '\n' ' ')"
if [ "$reload_5xx" -eq 0 ]; then ok "no 5xx during reload storm"; else hard "$reload_5xx 5xx responses during reload storm"; fi
if [ "$distinct" -eq 1 ]; then ok "all $RELOAD_CONCURRENCY reloads agreed on one status"; else warn "mixed status codes ($distinct distinct) — investigate but not fatal"; fi

S=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/status")
if [ "$S" = "200" ]; then ok "server still up after reload storm"; else hard "server unhealthy after reload storm (/api/status=$S)"; fi

# ============================================================================
# Scenario 3 — CRUD churn + RSS stability
# ============================================================================
section "CRUD churn — $CRUD_CYCLES add→delete cycles (RSS leak check)"

if [ -z "${SERVER_PID:-}" ]; then
    warn "SERVER_PID not set — RSS leak check skipped"
    track_rss=0
else
    track_rss=1
    rss_start=$(rss_kb_of "$SERVER_PID")
    echo "  RSS before: ${rss_start} KB (pid $SERVER_PID)"
fi

CRUD_FILE="$CONF_DIR/perf-crud.conf"
: > "$CRUD_FILE"
crud_fail=0
for i in $(seq 1 "$CRUD_CYCLES"); do
    v=$i
    # aa:bb:dd (NOT aa:bb:cc) so CRUD MACs never collide with the seed.conf
    # hosts from scenario 1: findHostsByMac scans ALL .conf in ConfigDir.
    mac=$(printf 'aa:bb:dd:%02x:%02x:%02x' $(( (v >> 16) & 0xFF )) $(( (v >> 8) & 0xFF )) $(( v & 0xFF )))
    ip="10.99.99.$(( i % 254 + 1 ))"
    sc=$(POST "$JWT" "/api/hosts" "{\"mac\":\"$mac\",\"ip\":\"$ip\",\"hostname\":\"crud$i\",\"file\":\"$CRUD_FILE\"}")
    [ "$sc" = "200" ] || { crud_fail=$((crud_fail + 1)); continue; }
    sc=$(DELETE "$JWT" "/api/hosts/$mac?file=$CRUD_FILE")
    [ "$sc" = "200" ] && continue || crud_fail=$((crud_fail + 1))
done
echo "  CRUD failures: $crud_fail / $CRUD_CYCLES"
if [ "$crud_fail" -eq 0 ]; then ok "all CRUD cycles succeeded"; else hard "$crud_fail CRUD cycles failed"; fi

if [ "$track_rss" -eq 1 ]; then
    rss_end=$(rss_kb_of "$SERVER_PID")
    delta_kb=$(( rss_end - rss_start ))
    delta_mb=$(( delta_kb / 1024 ))
    echo "  RSS after:  ${rss_end} KB | delta ${delta_mb} MB (leak-hard limit ${RSS_LEAK_HARD_MB} MB)"
    if   [ "$delta_mb" -ge "$RSS_LEAK_HARD_MB" ]; then hard "RSS grew ${delta_mb} MB — likely leak"
    elif [ "$delta_mb" -ge "$RSS_LEAK_MB" ];     then warn "RSS grew ${delta_mb} MB — watch but not fatal"
    else ok "RSS growth ${delta_mb} MB within budget"; fi
fi

# ============================================================================
# Scenario 4 — SSE endurance (N clients held for SSE_SECONDS)
# ============================================================================
section "SSE endurance — $SSE_CLIENTS clients x ${SSE_SECONDS}s on /api/events"

sse_pids=()
for _ in $(seq 1 "$SSE_CLIENTS"); do
    curl -sN -H "Authorization: Bearer $JWT" "$BASE/api/events" >/dev/null 2>&1 &
    sse_pids+=("$!")
done

sleep "$SSE_SECONDS"

alive=0
for p in "${sse_pids[@]}"; do kill -0 "$p" 2>/dev/null && alive=$((alive + 1)); done
for p in "${sse_pids[@]}"; do kill "$p" 2>/dev/null; done
wait 2>/dev/null || true

dropped=$(( SSE_CLIENTS - alive ))
drop_pct=$(( dropped * 100 / SSE_CLIENTS ))
echo "  alive=${alive}/${SSE_CLIENTS} dropped=${dropped} (${drop_pct}%) after ${SSE_SECONDS}s"
if [ "$drop_pct" -ge "$SSE_DROP_HARD_PCT" ]; then hard "SSE drop rate ${drop_pct}% ≥ hard limit ${SSE_DROP_HARD_PCT}%"
elif [ "$dropped" -gt 0 ];                    then warn "$dropped SSE clients dropped (within tolerance)"
else ok "all $SSE_CLIENTS SSE clients survived ${SSE_SECONDS}s"; fi

S=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/status")
if [ "$S" = "200" ]; then ok "server still up after SSE endurance"; else hard "server unhealthy after SSE endurance (/api/status=$S)"; fi

# ============================================================================
# Summary
# ============================================================================
printf "\n${CYAN}=== PERF SUMMARY ===${RESET}\n"
if [ "$HARD_FAIL" -gt 0 ]; then
    printf "${RED}perf: %d hard failure(s) — investigate.${RESET}\n" "$HARD_FAIL"
    exit 1
fi
printf "${GREEN}perf: no hard failures (warnings are informational).${RESET}\n"
exit 0
