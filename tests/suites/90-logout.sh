# tests/suites/90-logout.sh — POST /api/logout, JWT blacklist check.

if have_jwt; then
    section "logout"
    S=$(POST "$JWT" "/api/logout" "{}")
    check "POST /api/logout" 200 "$S" || true

    S=$(GET "$JWT" "/api/hosts")
    check "Old JWT after logout → 401 (blacklist works)" 401 "$S" || true
else
    skip "logout section (no JWT)"
fi
