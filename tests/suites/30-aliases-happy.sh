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

# tests/suites/30-aliases-happy.sh — A/CNAME/PTR/TXT happy paths + A12 underscore.
# Defines ALIAS_FILE (20-aliases.conf) reused by 31/32 suites.

ALIAS_FILE="$CONF_DIR/20-aliases.conf"

if require_jwt "DNS aliases — happy path" 6; then
    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"nas.local\",\"target\":\"10.0.0.5\",\"file\":\"$ALIAS_FILE\"}")
    check "Add A record" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"CNAME\",\"domain\":\"www.local\",\"target\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Add CNAME" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"PTR\",\"domain\":\"5.0.0.10.in-addr.arpa\",\"target\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Add PTR" 200 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"TXT\",\"domain\":\"_dmarc.local\",\"target\":\"v=DMARC1;p=reject\",\"file\":\"$ALIAS_FILE\"}")
    # A12: aliasDomainRegex rejects '_' in domain → breaks DMARC/DKIM/ACME.
    # Should accept — DNS RFC allows underscore in owner names for SRV/TXT/etc.
    check "A12: Add TXT with underscore domain → 200" 200 "$S" A12 || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"bad.local\",\"target\":\"not-an-ip\",\"file\":\"$ALIAS_FILE\"}")
    check "A with non-IP target → 400" 400 "$S" || true

    # P2.1: GET /api/aliases after the adds above — list must reflect the
    # written entries, not silently return [] with 200. Four aliases (A,
    # CNAME, PTR, TXT) were added; >=1 is the robust non-empty guard.
    S=$(GET "$JWT" "/api/aliases")
    check "GET /api/aliases → 200" 200 "$S" || true
    check_length "GET /api/aliases body non-empty" 1
fi
