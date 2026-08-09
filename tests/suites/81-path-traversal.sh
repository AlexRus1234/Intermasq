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

# tests/suites/81-path-traversal.sh — A11 path-traversal battery (9 vectors).

if require_jwt "path traversal (A11)" 9; then
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:f1\",\"file\":\"/etc/passwd\"}")
    check "Hosts file=/etc/passwd rejected" 400 "$S" || true

    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:f1?file=/etc/passwd")
    check "DELETE host file=/etc/passwd rejected" 400 "$S" || true

    S=$(POST "$JWT" "/api/aliases" "{\"type\":\"A\",\"domain\":\"evil.test\",\"target\":\"10.0.0.1\",\"file\":\"../../../tmp/x.conf\"}")
    check "Aliases file traversal rejected" 403 "$S" || true

    # NOTE: Go's net/http server cleans paths with `..` BEFORE Gin's router
    # sees them. So `/api/files/../../etc/passwd` is normalised to
    # `/etc/passwd` and never matches the `/api/files/:name` route — it
    # returns 404 NoRoute instead of reaching getFileHandler's explicit
    # traversal check. The explicit check is still valuable as defense in
    # depth (and for non-URL attack vectors), but via standard HTTP the
    # framework already protects us. Expect 404 here.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" --path-as-is -H "Authorization: Bearer $JWT" "$BASE/api/files/..%2F..%2Fetc%2Fpasswd")
    check "GET raw file traversal blocked (404 path-cleaned by Go HTTP)" 404 "$S" || true

    S=$(curl -s -o /dev/null -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"content":"x"}' "$BASE/api/files/passwd")
    check "PUT raw file non-.conf rejected" 403 "$S" || true

    S=$(POST "$JWT" "/api/history/restore" "{\"file\":\"/etc/hosts\",\"version\":\"20240101-000000\"}")
    check "History restore file=/etc/hosts rejected" 400 "$S" || true

    S=$(GET "$JWT" "/api/history?file=/etc/shadow")
    check "GET history file=/etc/shadow rejected" 400 "$S" || true

    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa:bb:cc:dd:ee:f2\",\"hostname\":\"a\\ndhcp-host=evil\",\"file\":\"$FILE\"}")
    check "Hostname with newline rejected" 400 "$S" || true
fi
