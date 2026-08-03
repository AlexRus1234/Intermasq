# tests/suites/40-config-files.sh — config file CRUD + raw PUT + dnsmasq --test validation.

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
    # P2.1: length-assert on the nested files array (not top-level length,
    # which would just count keys). 30-test.conf and 40-dhcp.conf were
    # created above; 30-test.conf is deleted below but only after this GET.
    check_length "GET /api/config has >=1 file" 1 '.files | length'

    S=$(GET "$JWT" "/api/files/30-test.conf")
    check "GET raw file" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"# test\ndomain-needed\nbogus-priv\n"}' "$BASE/api/files/30-test.conf")
    check "PUT raw file with valid content" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"# invalid\nport=abc\n"}' "$BASE/api/files/30-test.conf")
    # writeFileRaw runs `dnsmasq --test --conf-file=<path>`, so the invalid
    # `port=abc` is genuinely validated against the file we just wrote and
    # rejected with 400 + a dnsmasq error (A13 fixed).
    check "PUT with invalid dnsmasq syntax → 400" 400 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"file":"'"$CONF_DIR"'/30-test.conf"}' "$BASE/api/config/file")
    check "DELETE config file" 200 "$S" || true

    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X DELETE -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"file":"'"$CONF_DIR"'/30-test.conf"}' "$BASE/api/config/file")
    check "DELETE missing file → 404" 404 "$S" || true
fi
