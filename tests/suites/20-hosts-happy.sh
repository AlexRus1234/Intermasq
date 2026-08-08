# tests/suites/20-hosts-happy.sh — static host CRUD happy paths.
# Defines FILE (10-static.conf) reused by 21-25 suites.

FILE="$CONF_DIR/10-static.conf"

if require_jwt "static hosts — happy path" 12; then
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:01\",\"ip\":\"10.0.0.11\",\"hostname\":\"test1\",\"file\":\"$FILE\"}")
    check "Add host (full record)" 200 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:01\",\"ip\":\"10.0.0.99\",\"hostname\":\"dupmac\",\"file\":\"$FILE\"}")
    check "Add duplicate MAC → 409" 409 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:02\",\"ip\":\"10.0.0.11\",\"hostname\":\"dupip\",\"file\":\"$FILE\"}")
    check "Add duplicate IP → 409" 409 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:03\",\"hostname\":\"noip\",\"file\":\"$FILE\"}")
    check "Add host without IP (optional)" 200 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:04\",\"ip\":\"10.0.0.14\",\"file\":\"$FILE\"}")
    check "Add host without hostname (optional)" 200 "$S" || true

    # Tags
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:05\",\"ip\":\"10.0.0.15\",\"hostname\":\"tagged\",\"tags\":[\"set:iot\",\"set:guest\"],\"file\":\"$FILE\"}")
    check "Add host with set:iot,set:guest" 200 "$S" || true

    # Verify file content AFTER all 4 successful adds (ee:01, ee:03, ee:04, ee:05).
    # ee:02 was rejected as duplicate-IP so doesn't count.
    if [ -f "$FILE" ]; then
        LINES=$(grep -c "^dhcp-host=" "$FILE" || true)
        check "File has 4 dhcp-host lines (ee:01,ee:03,ee:04,ee:05)" 4 "$LINES" || true
    else
        check "File created" 4 0 || true
    fi

    if [ -f "$FILE" ] && grep -q "set:iot,set:guest" "$FILE"; then
        check "Tags written in file" 0 0 || true
    else
        check "Tags written in file" 0 1 || true
    fi

    # Invalid tag
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:06\",\"ip\":\"10.0.0.16\",\"hostname\":\"badtag\",\"tags\":[\"xyz:foo\"],\"file\":\"$FILE\"}")
    check "Add host with invalid tag → 400" 400 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:07\",\"ip\":\"10.0.0.17\",\"hostname\":\"tagmatcher\",\"tags\":[\"tag:guest\"],\"file\":\"$FILE\"}")
    check "Add host with tag matcher → 400" 400 "$S" || true
fi
