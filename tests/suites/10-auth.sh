# tests/suites/10-auth.sh — login flow, JWT/X-API-Key good+bad.

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
