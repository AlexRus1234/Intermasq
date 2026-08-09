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

# tests/suites/90-logout.sh — POST /api/logout, JWT blacklist check.

if have_jwt; then
    section "logout"
    S=$(POST "$JWT" "/api/logout" "{}")
    check "POST /api/logout" 200 "$S" || true

    S=$(GET "$JWT" "/api/hosts")
    check "Old JWT after logout → 401 (blacklist works)" 401 "$S" || true
else
    skip "logout section (no JWT)"
fi
