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

# tests/lib/auth.sh — JWT lifecycle helpers.
#
# have_jwt(): is there a usable JWT in the current run?
# require_jwt(section, count): if no JWT, skip the whole section with
# the given test count; otherwise print the section header.
# Depends on lib/state.sh (JWT, SKIP) and lib/common.sh (section).

# have_jwt: returns 0 if JWT is set and non-null, 1 otherwise.
have_jwt() { [ -n "${JWT:-}" ] && [ "${JWT:-}" != "null" ]; }

# require_jwt section_name approx_test_count: if no JWT, skip the whole section.
require_jwt() {
    local section="$1" count="$2"
    if ! have_jwt; then
        printf "\n${CYAN}=== %s ===${RESET}\n  ${BLUE}-- skipped${RESET} (no JWT: %s tests)\n" "$section" "$count"
        SKIP=$((SKIP + count))
        return 1
    fi
    section "$section"
    return 0
}
