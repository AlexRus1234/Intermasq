# tests/suites/40-config-files.sh — config file CRUD + raw PUT + A13 dnsmasq --test.

if require_jwt "config files" 10; then
    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"30-test.conf\",\"template\":\"empty\"}")
    check "Create 30-test.conf from empty template" 200 "$S" || true

    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"40-dhcp.conf\",\"template\":\"basic-dhcp\"}")
    check "Create 40-dhcp.conf from basic-dhcp template" 200 "$S" || true

    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"foo.txt\",\"template\":\"empty\"}")
    check "Reject non-.conf name → 400" 400 "$S" || true

    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"../../../tmp/evil.conf\",\"template\":\"empty\"}")
    check "Reject path-traversal name → 400" 400 "$S" || true

    S=$(GET "$JWT" "/api/config/templates")
    check "List config templates" 200 "$S" || true

    S=$(GET "$JWT" "/api/config")
    check "GET /api/config snapshot" 200 "$S" || true
    FILE_COUNT=$(body | jq '.files | length' 2>/dev/null || echo "?")
    echo "  config files in snapshot: $FILE_COUNT"

    S=$(GET "$JWT" "/api/files/30-test.conf")
    check "GET raw file" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"# test\ndomain-needed\nbogus-priv\n"}' "$BASE/api/files/30-test.conf")
    check "PUT raw file with valid content" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"# invalid\nport=abc\n"}' "$BASE/api/files/30-test.conf")
    # A13: writeFileRaw runs `dnsmasq --test` without --conf-file=<path>, so
    # dnsmasq tests its default config (not our newly-written file) and the
    # invalid `port=abc` slips through as 200. Once the call is changed to
    # `dnsmasq --test --conf-file=<path>` (or --conf-dir=$ConfigDir), this
    # will return 400 with a dnsmasq error.
    check "A13: PUT with invalid dnsmasq syntax → 400" 400 "$S" A13 || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"file":"'"$CONF_DIR"'/30-test.conf"}' "$BASE/api/config/file")
    check "DELETE config file" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"file":"'"$CONF_DIR"'/30-test.conf"}' "$BASE/api/config/file")
    check "DELETE missing file → 404" 404 "$S" || true
fi
