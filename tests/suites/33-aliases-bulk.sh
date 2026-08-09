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

# tests/suites/33-aliases-bulk.sh — POST /api/aliases/bulk.
# Validates: happy path with count, no_valid_entries, in-batch duplicate,
# unsafe target file.

if require_jwt "DNS aliases — bulk" 5; then
    # Happy: 3 fresh records to the existing alias file.
    S=$(POST "$JWT" "/api/aliases/bulk" "{\"file\":\"$ALIAS_FILE\",\"aliases\":[{\"type\":\"A\",\"domain\":\"bulk1.test\",\"target\":\"10.5.0.1\"},{\"type\":\"A\",\"domain\":\"bulk2.test\",\"target\":\"10.5.0.2\"},{\"type\":\"CNAME\",\"domain\":\"bulk3.test\",\"target\":\"bulk1.test\"}]}")
    check "Bulk-add 3 aliases → 200" 200 "$S" || true
    ALIAS_COUNT=$(body | jval .count)
    check "Bulk response has count=3" 3 "$ALIAS_COUNT" || true

    # Error: no valid entries (target is not an IP for A record).
    S=$(POST "$JWT" "/api/aliases/bulk" "{\"file\":\"$ALIAS_FILE\",\"aliases\":[{\"type\":\"A\",\"domain\":\"invalid.test\",\"target\":\"not-an-ip\"}]}")
    check "Bulk all invalid → 400 no_valid_entries" 400 "$S" || true

    # Error: duplicate domain within the batch.
    S=$(POST "$JWT" "/api/aliases/bulk" "{\"file\":\"$ALIAS_FILE\",\"aliases\":[{\"type\":\"A\",\"domain\":\"dup.test\",\"target\":\"10.6.0.1\"},{\"type\":\"A\",\"domain\":\"dup.test\",\"target\":\"10.6.0.2\"}]}")
    check "Bulk in-batch duplicate → 409" 409 "$S" || true

    # Error: unsafe target file.
    S=$(POST "$JWT" "/api/aliases/bulk" "{\"file\":\"/etc/evil.conf\",\"aliases\":[{\"type\":\"A\",\"domain\":\"x.test\",\"target\":\"10.7.0.1\"}]}")
    check "Bulk unsafe file → 403" 403 "$S" || true
fi
