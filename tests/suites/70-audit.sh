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

# tests/suites/70-audit.sh — audit log presence after actions.

if require_jwt "audit" 2; then
    S=$(GET "$JWT" "/api/audit")
    check "GET /api/audit" 200 "$S" || true
    AUDIT_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  audit entries: $AUDIT_COUNT"
    if [ "${AUDIT_COUNT:-0}" -gt 0 ]; then
        check "Audit log non-empty" 1 1 || true
    else
        check "Audit log non-empty" 1 0 || true
    fi
fi
