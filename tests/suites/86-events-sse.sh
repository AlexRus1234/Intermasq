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

# tests/suites/86-events-sse.sh — GET /api/events (SSE).
# P3.8: this endpoint had no smoke coverage. eventsHandler pushes an immediate
# `event:arp` frame on connect (internal/webapi/handlers.go:244 `c.SSEvent("arp", arp)`) before
# entering the streaming loop, so we can connect, grab the first event block,
# and assert an `event:` line arrived — without waiting for a deferred push.

if require_jwt "events SSE" 1; then
    # -sN: silent + no-buffer (streaming). head -n 5 closes the pipe after the
    # first event block → curl exits on SIGPIPE; `timeout 10` is the backstop
    # for a server that never sends the initial frame. `|| true` absorbs the
    # non-zero exit code from timeout/SIGPIPE — the grep below is the real
    # assertion.
    timeout 10 curl -sN -H "Authorization: Bearer $JWT" "$BASE/api/events" | head -n 5 > /tmp/smoke.sse 2>/dev/null || true
    if grep -q '^event:' /tmp/smoke.sse; then
        check "GET /api/events emits initial event" 1 1 || true
    else
        check "GET /api/events emits initial event" 1 0 || true
    fi
fi
