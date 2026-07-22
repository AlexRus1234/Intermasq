# tests/suites/21-hosts-bugs.sh — A3 (zero/broadcast MAC) + A4 (dash-MAC).

if require_jwt "static hosts — bug regressions" 3; then
    # A3: zero MAC should be rejected (currently accepted)
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"00:00:00:00:00:00\",\"ip\":\"10.0.0.99\",\"hostname\":\"zeromac\",\"file\":\"$FILE\"}")
    check "A3: zero MAC rejected → 400" 400 "$S" A3 || true

    # A3b: broadcast MAC should be rejected
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"ff:ff:ff:ff:ff:ff\",\"ip\":\"10.0.0.98\",\"hostname\":\"bcastmac\",\"file\":\"$FILE\"}")
    check "A3: broadcast MAC rejected → 400" 400 "$S" A3 || true

    # A4: dash separator should be normalized OR rejected (currently saved verbatim, breaks dnsmasq)
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa-bb-cc-dd-ee-07\",\"ip\":\"10.0.0.17\",\"hostname\":\"dashmac\",\"file\":\"$FILE\"}")
    check "A4: dash-MAC handled (rejected or normalized)" 400 "$S" A4 || true
fi
