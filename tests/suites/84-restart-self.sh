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

# tests/suites/84-restart-self.sh — POST /api/restart-self.
# P3.8: this endpoint had no smoke coverage. In ci-mode (which smoke assumes —
# see smoke.sh usage example `-ci-mode=true`) it returns 200
# {"status":"restarting"} WITHOUT actually restarting: the RestartSelf
# goroutine is gated on `if !ciMode` (internal/webapi/register.go), so the server stays up
# and subsequent suites keep running. Against a NON-ci binary this endpoint
# WOULD restart the server — do not run this suite against such a binary.

if require_jwt "restart-self" 1; then
    S=$(POST "$JWT" "/api/restart-self" "{}")
    check "POST /api/restart-self → 200 (ci-mode no-op)" 200 "$S" || true
fi
