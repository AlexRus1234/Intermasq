# Intermasq - Web panel for dnsmasq
# Copyright (C) 2026 AlexRus1234
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program. If not, see <https://www.gnu.org/licenses/>.

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
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@/tmp/smoke.backup.zip" "$BASE/api/backup/restore")
    check "Restore valid ZIP → 200" 200 "$S" || true

    # Error: no file field.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X POST -H "Authorization: Bearer $JWT" "$BASE/api/backup/restore")
    check "Restore no file → 400" 400 "$S" || true

    # Error: not a ZIP.
    echo "this is not a zip file" > /tmp/smoke.notzip
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@/tmp/smoke.notzip" "$BASE/api/backup/restore")
    check "Restore non-ZIP → 400" 400 "$S" || true

    rm -f /tmp/smoke.backup.zip /tmp/smoke.notzip
fi
