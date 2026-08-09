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

# tests/suites/21-hosts-bugs.sh — A3 (zero/broadcast MAC) + A4 (dash-MAC).
#
# These invalid-MAC hosts land in a DEDICATED file (not $FILE/10-static.conf)
# because host-add writes them without running `dnsmasq --test`, and once
# written they poison every history snapshot of whatever file they're in.
# 10-static.conf is restored (and dnsmasq-validated via --conf-file, A13
# fixed) by 51-history-diff-restore.sh, so it must contain only valid
# dhcp-host lines. Isolating the A3/A4 junk here keeps that restore green.

if require_jwt "static hosts — bug regressions" 3; then
    BUG_FILE="$CONF_DIR/19-bugs.conf"

    # A3: zero MAC should be rejected (currently accepted)
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"00:00:00:00:00:00\",\"ip\":\"10.0.0.99\",\"hostname\":\"zeromac\",\"file\":\"$BUG_FILE\"}")
    check "A3: zero MAC rejected → 400" 400 "$S" A3 || true

    # A3b: broadcast MAC should be rejected
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"ff:ff:ff:ff:ff:ff\",\"ip\":\"10.0.0.98\",\"hostname\":\"bcastmac\",\"file\":\"$BUG_FILE\"}")
    check "A3: broadcast MAC rejected → 400" 400 "$S" A3 || true

    # A4: dash separator is normalised to ':' on input (dnsmasq rejects dashes).
    # POST returns 200 and the file must contain the colon form, not the dash form.
    S=$(POST "$JWT" "/api/hosts" "{\"mac\":\"aa-bb-cc-dd-ee-07\",\"ip\":\"10.0.0.17\",\"hostname\":\"dashmac\",\"file\":\"$BUG_FILE\"}")
    check "A4: dash-MAC normalised → 200" 200 "$S" || true
    if [ -f "$BUG_FILE" ]; then
        if grep -q "aa:bb:cc:dd:ee:07" "$BUG_FILE" && ! grep -q "aa-bb-cc-dd-ee-07" "$BUG_FILE"; then
            check "A4: file stores colon form, not dash" 0 0 || true
        else
            check "A4: file stores colon form, not dash" 0 1 || true
        fi
    else
        skip "A4: file stores colon form, not dash (file missing)"
    fi
fi
