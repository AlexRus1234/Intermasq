# tests/suites/50-safety-backup-history.sh — backup ZIP, history list, rollback.

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
    S=$(POST "$JWT" "/api/rollback" "{\"file\":\"$FILE\"}")
    check "POST /api/rollback" 200 "$S" || true
    else
        skip "POST /api/rollback (no history yet)"
    fi
fi
