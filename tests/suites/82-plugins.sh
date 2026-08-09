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

# tests/suites/82-plugins.sh — GET /api/plugins + plugin proxy (Gap 6).
#
# The CI workflow installs the mock "hello" plugin into
# /etc/intermasq/plugins/hello/ before starting intermasq, so loadPlugins()
# picks it up. Here we verify both that intermasq lists it and that requests
# to /plugins/hello/* are reverse-proxied onto the plugin's unix socket.

if require_jwt "plugins" 3; then
    S=$(GET "$JWT" "/api/plugins")
    check "GET /api/plugins → 200" 200 "$S" || true
    echo "  body: $(body)"

    # Gap 6: the hello fixture must show up in the loaded-plugins list.
    if body | jval 'map(.id)' | grep -q '"hello"'; then
        check "hello plugin is loaded" 0 0 || true
    else
        check "hello plugin is loaded" 0 1 || true
    fi

    # Gap 6: /plugins/hello/health is proxied onto the plugin's socket.
    # Plugin routes are protected by the standard auth middleware.
    S=$(GET "$JWT" "/plugins/hello/health")
    check "GET /plugins/hello/health proxied → 200" 200 "$S" || true
    echo "  proxy body: $(body)"
fi
