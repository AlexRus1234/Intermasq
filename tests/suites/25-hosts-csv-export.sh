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

# tests/suites/25-hosts-csv-export.sh — CSV export endpoint.

if require_jwt "static hosts — CSV export" 1; then
    S=$(GET "$JWT" "/api/hosts/csv")
    check "CSV export" 200 "$S" || true
    # P2.1: CSV is plain text, not JSON — guard with wc -l via check_lines
    # (>= comparison; check itself is exact-equality and wrong here). At
    # least header + 1 data row. By now (suites 20/22/23 ran) there are
    # >=5 static hosts, so >=2 lines is the robust non-empty guard.
    check_lines "CSV export has header + >=1 data row" 2
fi
