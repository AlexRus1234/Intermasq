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

# tests/suites/10-auth.sh — login flow, JWT/X-API-Key good+bad.

section "auth"

S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"wrong\"}")
check "Login wrong password → 401" 401 "$S" || true

if have_jwt; then
    S=$(GET "$JWT" "/api/hosts")
    check "GET /api/hosts with valid JWT" 200 "$S" || true
else
    skip "GET /api/hosts with valid JWT"
fi

S=$(GET "invalid.jwt.token" "/api/hosts")
check "GET /api/hosts with garbage JWT → 401" 401 "$S" || true

S=$(KGET "$SECRET" "/api/hosts")
check "GET /api/hosts with X-API-Key" 200 "$S" || true

S=$(KGET "wrong-secret" "/api/hosts")
check "GET /api/hosts with wrong X-API-Key → 401" 401 "$S" || true
