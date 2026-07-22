# tests/suites/70-audit.sh — audit log presence after actions.

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
