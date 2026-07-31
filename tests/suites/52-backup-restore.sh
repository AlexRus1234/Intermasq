# tests/suites/52-backup-restore.sh — POST /api/backup/restore.
# Validates: round-trip (download ZIP, upload it back), no file, non-ZIP data.

if require_jwt "backup restore" 4; then
    # Download a fresh backup ZIP first.
    S=$(GET "$JWT" "/api/backup")
    check "GET /api/backup (for restore) → 200" 200 "$S" || true
    cp /tmp/smoke.body /tmp/smoke.backup.zip

    # Happy: upload the same ZIP back. All .conf files in the archive are
    # restored. restoreBackupZip still runs bare `dnsmasq --test` (whole-
    # config, NOT per-file --conf-file) — A14: this evaluates dnsmasq's
    # *default* conf path, not the restored files. On dnsmasq ≥2.90 the
    # missing default conf is a warning (passes); on ≤2.86 it is exit 1
    # → 400 dnsmasq_test_failed. Tagged A14 so the compat-matrix pipeline
    # stays yellow on 2.80/2.86 until the bug is fixed in backup.go:119.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@/tmp/smoke.backup.zip" "$BASE/api/backup/restore")
    check "Restore valid ZIP → 200" 200 "$S" A14 || true

    # Error: no file field.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST -H "Authorization: Bearer $JWT" "$BASE/api/backup/restore")
    check "Restore no file → 400" 400 "$S" || true

    # Error: not a ZIP.
    echo "this is not a zip file" > /tmp/smoke.notzip
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@/tmp/smoke.notzip" "$BASE/api/backup/restore")
    check "Restore non-ZIP → 400" 400 "$S" || true

    rm -f /tmp/smoke.backup.zip /tmp/smoke.notzip
fi
