# tests/suites/52-backup-restore.sh — POST /api/backup/restore.
# Validates: round-trip (download ZIP, upload it back), no file, non-ZIP data.

if require_jwt "backup restore" 4; then
    # Download a fresh backup ZIP first.
    S=$(GET "$JWT" "/api/backup")
    check "GET /api/backup (for restore) → 200" 200 "$S" || true
    cp /tmp/smoke.body /tmp/smoke.backup.zip

    # Happy: upload the same ZIP back. All .conf files in the archive are
    # restored. restoreBackupZip runs `dnsmasq --test --conf-file=<path>`
    # per restored file (A14 fixed in predrel-test-remediation-P1,
    # 2026-08-02 — same canonical pattern as A13 in writeFileRaw/
    # writeConfigWithTest/restoreHistoryVersion), validating exactly what
    # the restore wrote. Returns 200 on any dnsmasq ≥2.86.
    #
    # On dnsmasq 2.80 the restore can still fail with 400 dnsmasq_test_failed
    # if the backup contains 10-static.conf with dhcp-host tag-set content
    # that 2.80 rejects at --test — this is the SAME root cause as A15
    # (history-restore path, suite 51). A14 previously masked it (bare --test
    # failed on missing default config before ever reaching the file content).
    # Tagged A15 + body_pattern so: on ≥2.86 → PASS; on 2.80 → KNOWN-fail
    # (body matches 'dnsmasq_test_failed'); any unrelated regression → hard FAIL.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@/tmp/smoke.backup.zip" "$BASE/api/backup/restore")
    check "Restore valid ZIP → 200" 200 "$S" A15 'dnsmasq_test_failed' || true

    # Error: no file field.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST -H "Authorization: Bearer $JWT" "$BASE/api/backup/restore")
    check "Restore no file → 400" 400 "$S" || true

    # Error: not a ZIP.
    echo "this is not a zip file" > /tmp/smoke.notzip
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@/tmp/smoke.notzip" "$BASE/api/backup/restore")
    check "Restore non-ZIP → 400" 400 "$S" || true

    rm -f /tmp/smoke.backup.zip /tmp/smoke.notzip
fi
