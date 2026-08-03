# tests/suites/26-hosts-bulk-move.sh — POST /api/hosts/bulk-move.
# Moves a host from FILE (set by 20-hosts-happy.sh) into a fresh target file.
# Validates: happy path, file-content round-trip, no_hosts, unsafe target,
# same_file rejection.

if require_jwt "static hosts — bulk-move" 7; then
    MOVED_FILE="$CONF_DIR/15-moved.conf"

    # P2.2 self-seed: recreate ee:10 in $FILE so this suite no longer
    # depends on 23-hosts-csv.sh having run. The bulk-move below reads ee:10
    # from $FILE as its source, so the seed must land in $FILE (not a
    # separate file). 200 (created) and 409 (already present from 23) are
    # both acceptable; any other status is surfaced as a real failure.
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:10\",\"ip\":\"10.0.0.20\",\"hostname\":\"csv1\",\"file\":\"$FILE\"}")
    case "$S" in
        200|409) ;;
        *) check "self-seed ee:10 (expected 200|409)" 200 "$S" || true ;;
    esac

    # Happy: move ee:10 into a new target file.
    S=$(POST "$JWT" "/api/hosts/bulk-move" "{\"target\":\"$MOVED_FILE\",\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:10\",\"file\":\"$FILE\"}]}")
    check "Bulk-move 1 host → 200" 200 "$S" || true
    MOVED_COUNT=$(body | jval .moved)
    check "Bulk-move moved=1" 1 "$MOVED_COUNT" || true

    # Verify the host is now in the target file.
    if [ -f "$MOVED_FILE" ] && grep -q "aa:bb:cc:dd:ee:10" "$MOVED_FILE"; then
        check "Moved host present in target file" 0 0 || true
    else
        check "Moved host present in target file" 0 1 || true
    fi
    # Verify it's gone from source.
    if grep -q "aa:bb:cc:dd:ee:10" "$FILE"; then
        check "Moved host removed from source file" 0 1 || true
    else
        check "Moved host removed from source file" 0 0 || true
    fi

    # Error: empty hosts list.
    S=$(POST "$JWT" "/api/hosts/bulk-move" "{\"target\":\"$MOVED_FILE\",\"hosts\":[]}")
    check "Bulk-move empty hosts → 400" 400 "$S" || true

    # Error: target outside ConfigDir.
    S=$(POST "$JWT" "/api/hosts/bulk-move" "{\"target\":\"/etc/evil.conf\",\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:99\",\"file\":\"$FILE\"}]}")
    check "Bulk-move unsafe target → 403" 403 "$S" || true

    # Error: source file == target.
    S=$(POST "$JWT" "/api/hosts/bulk-move" "{\"target\":\"$FILE\",\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:11\",\"file\":\"$FILE\"}]}")
    check "Bulk-move same file → 400" 400 "$S" || true
fi
