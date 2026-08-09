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

# tests/suites/23-hosts-csv.sh — CSV import path with count field (A6 regression).

if require_jwt "static hosts — CSV import (A6)" 2; then
    CSV=$(mktemp)
    cat > "$CSV" <<EOF
mac,ip,hostname
aa:bb:cc:dd:ee:10,10.0.0.20,csv1
aa:bb:cc:dd:ee:11,10.0.0.21,csv2
aa:bb:cc:dd:ee:12,10.0.0.22,csv3
EOF
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@$CSV" -F "target_file=$FILE" "$BASE/api/hosts/csv")
    check "CSV import 3 hosts" 200 "$S" || true
    CSV_COUNT=$(body | jval .count)
    check "A6: CSV response has count=3" 3 "$CSV_COUNT" A6 || true
    rm -f "$CSV"
fi
