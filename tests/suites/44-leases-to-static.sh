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

# tests/suites/44-leases-to-static.sh — POST /api/leases/to-static.
# P3.8: this endpoint had no smoke coverage. bulkLeaseToStaticHandler writes
# one dhcp-host= line per selected lease directly to a file (it intentionally
# does NOT run `dnsmasq --test` — see internal/webapi/handlers.go:129 comment), so it is safe
# to exercise against a fresh file. We POST two synthetic leases and assert
# 200 + count == 2.

if require_jwt "leases to-static" 2; then
    TOSTATIC_FILE="$CONF_DIR/e2e-tostatic.conf"
    S=$(POST "$JWT" "/api/leases/to-static" "{\"file\":\"$TOSTATIC_FILE\",\"leases\":[{\"mac\":\"aa:cc:00:00:00:01\",\"ip\":\"10.99.44.1\",\"hostname\":\"ts1\"},{\"mac\":\"aa:cc:00:00:00:02\",\"ip\":\"10.99.44.2\",\"hostname\":\"ts2\"}]}")
    check "POST /api/leases/to-static → 200" 200 "$S" || true
    COUNT=$(body | jval .count)
    echo "  to-static count: $COUNT"
    check "to-static converted 2 leases" 2 "$COUNT" || true
fi
