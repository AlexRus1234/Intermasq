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

# tests/suites/51-history-diff-restore.sh — GET /api/history/diff +
# POST /api/history/restore.
# Validates: diff happy round-trip, missing params, unknown version,
# restore happy round-trip, restore missing/invalid version.

if require_jwt "history diff + restore" 7; then
    # Discover the newest version of FILE (history populated by earlier suites).
    S=$(GET "$JWT" "/api/history?file=$FILE")
    check "GET /api/history for diff/restore → 200" 200 "$S" || true
    NEWEST_V=$(body | jval '.versions[0].version')
    echo "  newest version: $NEWEST_V"

    # Error: diff without required `from` param.
    S=$(GET "$JWT" "/api/history/diff?file=$FILE")
    check "Diff without from → 400" 400 "$S" || true

    # Error: diff with non-existent version (regex-valid, file-missing).
    S=$(GET "$JWT" "/api/history/diff?file=$FILE&from=19990101-000000")
    check "Diff unknown version → 404" 404 "$S" || true

    if [ -n "$NEWEST_V" ] && [ "$NEWEST_V" != "null" ]; then
        # Happy: diff newest vs current.
        S=$(GET "$JWT" "/api/history/diff?file=$FILE&from=$NEWEST_V")
        check "Diff newest vs current → 200" 200 "$S" || true
        DIFF_LEN=$(body | jval '.diff' | wc -c)
        echo "  diff bytes: $DIFF_LEN"

        # Happy: restore that version. restoreHistoryVersion runs
        # `dnsmasq --test --conf-file=<path>` after writing (A13 fixed); the
        # restored version is a previously-saved snapshot of valid dhcp-host
        # lines, so it validates cleanly and returns 200.
        #
        S=$(POST "$JWT" "/api/history/restore" "{\"file\":\"$FILE\",\"version\":\"$NEWEST_V\"}")
        check "Restore known version → 200" 200 "$S" || true
    else
        skip "Diff/restore happy path (no history versions)"
    fi

    # Error: restore with missing version param.
    S=$(POST "$JWT" "/api/history/restore" "{\"file\":\"$FILE\"}")
    check "Restore missing version → 400" 400 "$S" || true

    # Error: restore with invalid version format (regex check inside
    # restoreHistoryVersion → returns "invalid_version" → 500 restore_error).
    S=$(POST "$JWT" "/api/history/restore" "{\"file\":\"$FILE\",\"version\":\"not-a-date\"}")
    check "Restore invalid version format → 500" 500 "$S" || true
fi
