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

# tests/suites/00-preflight.sh — clean CONF_DIR, status probe, JWT obtain.
# Source order: first. Sets the shared JWT slot used by all later suites.

section "pre-flight"

rm -rf "$CONF_DIR"
mkdir -p "$CONF_DIR"
S=$(PGET "/api/status")
check "GET /api/status (no auth)" 200 "$S" || true
echo "  status body: $(body)"

# Read setup_required from body — NOTE: do NOT pipe PGET into jval,
# PGET returns HTTP code on stdout (body goes to /tmp/smoke.body file).
PGET "/api/status" >/dev/null
USERS=$(body | jval .setup_required)
echo "  setup_required=$USERS"

if [ "$USERS" = "true" ]; then
    S=$(PPOST "/api/setup" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
    check "POST /api/setup (create admin)" 200 "$S" || true
    JWT=$(body | jval .token)
else
    S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
    check "POST /api/login (existing admin)" 200 "$S" || true
    JWT=$(body | jval .token)
fi

if ! have_jwt; then
    fatal "no JWT obtained (setup/login failed) — most auth-required tests will be skipped"
fi
have_jwt && echo "  JWT: ${JWT:0:30}..."
