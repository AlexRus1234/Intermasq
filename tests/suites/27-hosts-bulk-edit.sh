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

# tests/suites/27-hosts-bulk-edit.sh — POST /api/hosts/bulk-edit.
# Applies an IP-prefix transform (10.0.0 → 10.0.1) to ee:11.
# NOTE: the host must have a non-empty hostname — bulkEditHandler validates
# the resulting hostname via validHostname() even if the hostname isn't
# being transformed, and validHostname("") returns false. ee:04 (created
# without hostname by 20-hosts-happy.sh) cannot be used here.
# Validates: happy path, file-content verification, no_hosts, partial prefix,
# unknown host.

if require_jwt "static hosts — bulk-edit" 5; then
    # P2.2 self-seed: recreate ee:11 (csv2 / 10.0.0.21) in $FILE so this
    # suite no longer depends on 23-hosts-csv.sh having run. The IP must
    # keep the 10.0.0 prefix (the transform below rewrites that prefix) and
    # the hostname must be non-empty ("csv2") because bulkEditHandler
    # validates it via validHostname() even when not transforming it. 200
    # (created) and 409 (already present from 23) are both acceptable.
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:11\",\"ip\":\"10.0.0.21\",\"hostname\":\"csv2\",\"file\":\"$FILE\"}")
    case "$S" in
        200|409) ;;
        *) check "self-seed ee:11 (expected 200|409)" 200 "$S" || true ;;
    esac

    # Happy: transform ee:11 (csv2, hostname="csv2", IP=10.0.0.21)
    # → 10.0.1.21 via prefix replace.
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:11\",\"file\":\"$FILE\"}],\"ip_transform\":{\"old_prefix\":\"10.0.0\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit prefix transform → 200" 200 "$S" || true
    UPDATED_COUNT=$(body | jval .updated)
    check "Bulk-edit updated=1" 1 "$UPDATED_COUNT" || true

    # Verify the IP actually changed in the file.
    if grep -q "10.0.1.21" "$FILE"; then
        check "Edited host has new IP in file" 0 0 || true
    else
        check "Edited host has new IP in file" 0 1 || true
    fi

    # Error: empty hosts.
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[],\"ip_transform\":{\"old_prefix\":\"10.0.0\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit empty hosts → 400" 400 "$S" || true

    # Error: prefix mismatch (only new_prefix provided).
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:11\",\"file\":\"$FILE\"}],\"ip_transform\":{\"old_prefix\":\"\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit partial prefix → 400" 400 "$S" || true

    # Error: host not found in source file.
    S=$(POST "$JWT" "/api/hosts/bulk-edit" "{\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ff:99\",\"file\":\"$FILE\"}],\"ip_transform\":{\"old_prefix\":\"10.0.0\",\"new_prefix\":\"10.0.1\"}}")
    check "Bulk-edit unknown host → 404" 404 "$S" || true
fi
