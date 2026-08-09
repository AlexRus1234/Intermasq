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

# tests/suites/50-safety-backup-history.sh — backup ZIP, history list, rollback.

if require_jwt "safety — backup / history" 3; then
    S=$(GET "$JWT" "/api/backup")
    check "GET /api/backup (ZIP download)" 200 "$S" || true
    ZIP_SIZE=$(wc -c < /tmp/smoke.body)
    echo "  backup zip size: $ZIP_SIZE bytes"

    S=$(GET "$JWT" "/api/history?file=$FILE")
    check "GET /api/history for static file" 200 "$S" || true
    # P2.1: length-assert on the nested versions array. Prior host mutations
    # (suites 20/22) snapshotted pre-write state via createLocalBackup, so at
    # least one version must exist here.
    check_length "GET /api/history has >=1 version" 1 '.versions | length'
    HIST_COUNT=$(body | jq '.versions | length' 2>/dev/null || echo "0")

    # Rollback test — only attempt if history exists
    if [ "${HIST_COUNT:-0}" -gt 0 ]; then
    S=$(POST "$JWT" "/api/rollback" "{\"file\":\"$FILE\"}")
    check "POST /api/rollback" 200 "$S" || true
    else
        skip "POST /api/rollback (no history yet)"
    fi
fi
