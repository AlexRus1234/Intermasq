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

# tests/suites/31-aliases-bugs.sh — A2 duplicate-allowed regression.

if require_jwt "DNS aliases — A2 regression" 2; then
    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"nas.local\",\"target\":\"10.0.0.99\",\"file\":\"$ALIAS_FILE\"}")
    check "A2: duplicate A same file → 409" 409 "$S" A2 || true

    if [ -f "$ALIAS_FILE" ]; then
        DUP_COUNT=$(grep -c "^address=/nas\.local/" "$ALIAS_FILE" || true)
        check "A2: file has exactly 1 nas.local A record" 1 "$DUP_COUNT" A2 || true
    else
        skip "A2: file has exactly 1 nas.local A record (file missing)"
    fi
fi
