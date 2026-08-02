#!/bin/bash
# tests/l5/vm-check.sh — L5 functional assertions на живой ВМ (ОБА инстанса).
# Гоняется ВНУТРИ ВМ ПОСЛЕ provision.sh. Покрывает то, что fake-тесты не могут:
#   root       (18081, User=root)            → UseSudo=false ветки caller'а
#   rootless   (18082, User=intermasq+sudo)  → UseSudo=true  ветки caller'a
#     detect (sudo is-active probe) + IsActive + Restart(dnsmasq) via sudo +
#     RestartSelf (sudo systemctl/rc-service restart intermasq — hardcoded name
#     в system.go:61, поэтому рестартит root-sibling; PID-change доказывает выполнение).
set -u
EXPECTED_INIT="${EXPECTED_INIT:-}"
PASS=0; FAIL=0; SKIP=0
ok(){ printf '  \033[32m✓\033[0m %s\n' "$1"; PASS=$((PASS+1)); }
bad(){ printf '  \033[31m✗ %s\033[0m\n' "$1"; FAIL=$((FAIL+1)); }
warn(){ printf '  \033[33m- %s\033[0m\n' "$1"; SKIP=$((SKIP+1)); }

# PID конкретного сервиса (не pidof — оба инстанса запущены как intermasq-ci).
svc_pid(){
    if command -v systemctl >/dev/null 2>&1 && [ "$(cat /proc/1/comm)" = "systemd" ]; then
        systemctl show -p MainPID --value "$1" 2>/dev/null
    else
        cat "/run/$1.pid" 2>/dev/null
    fi
}
pid_of(){ pidof "$1" 2>/dev/null | tr ' ' '\n' | head -n1; }

# --- один «прогон» для инстанса -------------------------------------------
# args: TAG PORT SVC LOG EXPECT_ROLE
run_instance(){
    local TAG="$1" PORT="$2" SVC="$3" LOG="$4" ROLE="$5"
    local BASE="http://127.0.0.1:$PORT"
    echo
    echo "===== $TAG  ($BASE, svc=$SVC, expect role='$ROLE') ====="

    echo "[detect] [INIT] System: из $LOG"
    local DET; DET=$(grep -m1 '\[INIT\] System:' "$LOG" 2>/dev/null | sed 's/.*\[INIT\] System: //')
    echo "    detected: ${DET:-<none>}"
    if [ -z "$DET" ]; then bad "no [INIT] line in $LOG"; return; fi
    ok "detect produced: $DET"
    if [ -n "$EXPECTED_INIT" ]; then
        case "$DET" in "$EXPECTED_INIT "*|"$EXPECTED_INIT") ok "init=$EXPECTED_INIT" ;; *) bad "init: want '$EXPECTED_INIT', got '$DET'" ;; esac
    fi
    case "$DET" in *"$ROLE"*) ok "role matches '$ROLE'" ;; *) bad "role: want '*$ROLE*', got '$DET'" ;; esac

    echo "[auth] setup/login"
    local RESP JWT
    RESP=$(curl -s -m 5 -X POST "$BASE/api/setup" -H "Content-Type: application/json" -d '{"username":"admin","password":"pass1234"}')
    JWT=$(printf '%s' "$RESP" | grep -oE '"token":"[^"]*"' | sed 's/"token":"//;s/"$//')
    if [ -z "$JWT" ]; then
        RESP=$(curl -s -m 5 -X POST "$BASE/api/login" -H "Content-Type: application/json" -d '{"username":"admin","password":"pass1234"}')
        JWT=$(printf '%s' "$RESP" | grep -oE '"token":"[^"]*"' | sed 's/"token":"//;s/"$//')
    fi
    [ -n "$JWT" ] && ok "JWT obtained" || { bad "no JWT"; return; }

    echo "[status] GET /api/status → dnsmasq_active (real IsActive via init)"
    local BODY; BODY=$(curl -s -m 4 "$BASE/api/status")
    echo "    body: $BODY"
    case "$BODY" in *'"dnsmasq_active":true'*) ok "dnsmasq_active=true (IsActive via real init)" ;; *) bad "dnsmasq_active not true" ;; esac

    echo "[reload] POST /api/reload → реальный рестарт dnsmasq (смена PID)"
    local D1 D2 RC
    D1=$(pid_of dnsmasq); echo "    dnsmasq PID before: ${D1:-<none>}"
    RC=$(curl -s -m 8 -o /dev/null -w "%{http_code}" -X POST "$BASE/api/reload" -H "Authorization: Bearer $JWT")
    echo "    /api/reload HTTP $RC"
    [ "$RC" = 200 ] || bad "/api/reload HTTP=$RC"
    sleep 2
    D2=$(pid_of dnsmasq); echo "    dnsmasq PID after:  ${D2:-<none>}"
    if [ -n "$D1" ] && [ -n "$D2" ] && [ "$D1" != "$D2" ]; then ok "dnsmasq restarted (PID $D1→$D2)"; else bad "dnsmasq NOT restarted"; fi
    if command -v dig >/dev/null 2>&1; then
        local ANS; ANS=$(dig @10.5.0.1 probe.l5.test +short +time=2 +tries=1 2>/dev/null | head -n1)
        [ "$ANS" = "10.5.0.9" ] && ok "dnsmasq functional (probe→10.5.0.9)" || warn "dig probe='${ANS:-<>}'"
    else warn "dig absent"; fi

    # RestartSelf: hardcoded "intermasq" (system.go:61) → всегда рестартит svc intermasq.
    # Для root = self; для rootless = root-sibling (доказывает что sudo-ветка выполнилась).
    echo "[restart-self] POST /api/restart-self → svc 'intermasq' PID change"
    local T1 T2
    T1=$(svc_pid intermasq); echo "    svc intermasq PID before: ${T1:-<none>}"
    RC=$(curl -s -m 5 -o /dev/null -w "%{http_code}" -X POST "$BASE/api/restart-self" -H "Authorization: Bearer $JWT")
    echo "    /api/restart-self HTTP $RC"
    [ "$RC" = 200 ] || bad "/api/restart-self HTTP=$RC"
    local BACK=0 i
    for i in $(seq 1 20); do sleep 1; curl -s -m 2 "http://127.0.0.1:18081/api/status" >/dev/null 2>&1 && { BACK=$i; break; }; done
    T2=$(svc_pid intermasq); echo "    svc intermasq PID after:  ${T2:-<none>} (root back after ${BACK}s)"
    if [ -n "$T1" ] && [ -n "$T2" ] && [ "$T1" != "$T2" ] && [ "$T2" != "0" ]; then ok "intermasq restarted (PID $T1→$T2)"; else bad "intermasq NOT restarted"; fi
}

echo "=== L5 vm-check (EXPECTED_INIT=${EXPECTED_INIT:-<any>}); 2 инстанса ==="
# Сначала root (его restart-self рестартит intermasq=self), затем rootless
# (его restart-self тоже бьёт по hardcoded "intermasq" = root sibling).
run_instance "ROOT    " 18081 intermasq         /var/log/intermasq.log    "root"
run_instance "ROOTLESS" 18082 intermasq-rootless /var/log/intermasq-rw.log "via sudo"

echo
echo "=== RESULT: PASS=$PASS FAIL=$FAIL SKIP=$SKIP ==="
[ "$FAIL" = 0 ]
