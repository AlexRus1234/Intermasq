# tests/suites/21-hosts-bugs.sh — A3 (zero/broadcast MAC) + A4 (dash-MAC).
#
# These invalid-MAC hosts land in a DEDICATED file (not $FILE/10-static.conf)
# because host-add writes them without running `dnsmasq --test`, and once
# written they poison every history snapshot of whatever file they're in.
# 10-static.conf is restored (and dnsmasq-validated via --conf-file, A13
# fixed) by 51-history-diff-restore.sh, so it must contain only valid
# dhcp-host lines. Isolating the A3/A4 junk here keeps that restore green.

if require_jwt "static hosts — bug regressions" 3; then
    BUG_FILE="$CONF_DIR/19-bugs.conf"

    # A3: zero MAC should be rejected (currently accepted)
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"00:00:00:00:00:00\",\"ip\":\"10.0.0.99\",\"hostname\":\"zeromac\",\"file\":\"$BUG_FILE\"}")
    check "A3: zero MAC rejected → 400" 400 "$S" A3 || true

    # A3b: broadcast MAC should be rejected
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"ff:ff:ff:ff:ff:ff\",\"ip\":\"10.0.0.98\",\"hostname\":\"bcastmac\",\"file\":\"$BUG_FILE\"}")
    check "A3: broadcast MAC rejected → 400" 400 "$S" A3 || true

    # A4: dash separator should be normalized OR rejected (currently saved verbatim, breaks dnsmasq)
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa-bb-cc-dd-ee-07\",\"ip\":\"10.0.0.17\",\"hostname\":\"dashmac\",\"file\":\"$BUG_FILE\"}")
    check "A4: dash-MAC handled (rejected or normalized)" 400 "$S" A4 || true
fi
