# tests/suites/31-aliases-bugs.sh — A2 duplicate-allowed regression.

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
