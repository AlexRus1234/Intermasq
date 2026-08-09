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

# tests/suites/24-hosts-bulk.sh — bulk-add hosts via JSON, count regression (A6).

if require_jwt "static hosts — bulk add (no count regression)" 2; then
    S=$(POST "$JWT" "/api/hosts/bulk" "{\"file\":\"$FILE\",\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:20\",\"ip\":\"10.0.0.30\",\"hostname\":\"bulk1\"},{\"mac\":\"aa:bb:cc:dd:ee:21\",\"ip\":\"10.0.0.31\",\"hostname\":\"bulk2\"}]}")
    check "Bulk add 2 hosts → 200" 200 "$S" || true
    BULK_COUNT=$(body | jval .count)
    check "Bulk JSON response has count field" 2 "$BULK_COUNT" A6 || true
fi
