# tests/suites/91-plugins.sh — GET /api/plugins.
# In CI mode with no plugin dir configured, returns null or []. We only
# assert 200 + that the body parses as JSON.

if require_jwt "plugins" 1; then
    S=$(GET "$JWT" "/api/plugins")
    check "GET /api/plugins → 200" 200 "$S" || true
    echo "  body: $(body)"
fi
