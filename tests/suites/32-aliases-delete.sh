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

# tests/suites/32-aliases-delete.sh — delete A + second-delete (A2-dependent) + PTR/TXT.

if require_jwt "DNS aliases — delete" 3; then
    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"A\",\"domain\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete A record" 200 "$S" || true

    # Second delete: depends on A2 being fixed. While A2 allows duplicates,
    # there are 2 nas.local A records in file, so second delete finds the
    # other one and returns 200 instead of 404. Mark as KNOWN(A2) — will
    # become a clean pass once A2 is fixed.
    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"A\",\"domain\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete again → 404 (depends on A2 fix)" 404 "$S" A2 || true

    # PTR/TXT are creatable via the API, so they must be deletable too. The
    # delete handler used to accept only A/CNAME — that was the bug.
    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"PTR\",\"domain\":\"5.0.0.10.in-addr.arpa\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete PTR → 200 (PTR/TXT now deletable)" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"TXT\",\"domain\":\"_dmarc.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete TXT → 200 (PTR/TXT now deletable)" 200 "$S" || true
fi
