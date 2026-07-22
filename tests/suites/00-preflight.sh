# tests/suites/00-preflight.sh — clean CONF_DIR, status probe, JWT obtain.
# Source order: first. Sets the shared JWT slot used by all later suites.

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
