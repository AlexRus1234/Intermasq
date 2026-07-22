# tests/suites/27-hosts-bulk-edit.sh — POST /api/hosts/bulk-edit.
# Applies an IP-prefix transform (10.0.0 → 10.0.1) to ee:04.
# Validates: happy path, file-content verification, no_hosts, partial prefix,
# unknown host.

if require_jwt "static hosts — bulk-edit" 5; then
    # Happy: transform ee:04 from 10.0.0.14 → 10.0.1.14 via prefix replace.
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:04\",\"file\":\"$FILE\"}],\"ip_transform\":{\"old_prefix\":\"10.0.0\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit prefix transform → 200" 200 "$S" || true
    UPDATED_COUNT=$(body | jval .updated)
    check "Bulk-edit updated=1" 1 "$UPDATED_COUNT" || true

    # Verify the IP actually changed in the file.
    if grep -q "10.0.1.14" "$FILE"; then
        check "Edited host has new IP in file" 0 0 || true
    else
        check "Edited host has new IP in file" 0 1 || true
    fi

    # Error: empty hosts.
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[],\"ip_transform\":{\"old_prefix\":\"10.0.0\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit empty hosts → 400" 400 "$S" || true

    # Error: prefix mismatch (only new_prefix provided).
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:04\",\"file\":\"$FILE\"}],\"ip_transform\":{\"old_prefix\":\"\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit partial prefix → 400" 400 "$S" || true

    # Error: host not found in source file.
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ff:99\",\"file\":\"$FILE\"}],\"ip_transform\":{\"old_prefix\":\"10.0.0\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit unknown host → 404" 404 "$S" || true
fi
