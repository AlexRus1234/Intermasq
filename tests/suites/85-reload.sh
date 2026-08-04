# tests/suites/85-reload.sh — POST /api/reload.
# P3.8: this endpoint had no smoke coverage. reloadHandler runs `dnsmasq --test`
# + a restart via the init-system caller. On a CI host with a working dnsmasq
# it returns 200 {"status":"reloaded"}; without one (or if the restart caller
# is the NoneCaller / no init system) it returns 400 {"error":"reload_error"}.
# Both are acceptable shapes here — only an unrelated 401/500 would indicate a
# regression worth failing on.

if require_jwt "reload" 1; then
    S=$(POST "$JWT" "/api/reload" "{}")
    # Loosen to 200 (reloaded) OR 400 (reload_error: no/failed dnsmasq). The
    # pass branch passes exp==got so `check` records PASS and echoes which
    # status fired; the fail branch uses a sentinel that never matches.
    if [ "$S" = "200" ] || [ "$S" = "400" ]; then
        check "POST /api/reload → 200 (ok) or 400 (no dnsmasq)" "$S" "$S" || true
    else
        check "POST /api/reload → 200 (ok) or 400 (no dnsmasq)" "200|400" "$S" || true
    fi
fi
