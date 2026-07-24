#!/usr/bin/env bash
# tests/fixtures/gen-hosts.sh — seed fixture generator for perf tests (Gap 5).
#
# Writes <count> dhcp-host= lines into <out.conf>, mirroring the exact format
# the app itself writes (dhcp-host=MAC,IP,hostname) so parseDhcpHostLine reads
# them back without special-casing. MACs/IPs are derived from a counter so the
# dataset is deterministic and conflict-free for any count up to ~16M.
#
# Usage:   gen-hosts.sh <out.conf> <count> [mac-prefix]
# Example: gen-hosts.sh /tmp/perf-conf/seed.conf 200
#
# IPs walk 10.0.X.Y with X,Y in 1..254 (never .0 / .255). MAC suffix bytes
# come from (index+1) so the first entry is never the all-zero MAC that the
# panel would reject.

set -euo pipefail

out="${1:?usage: gen-hosts.sh <out.conf> <count> [mac-prefix]}"
n="${2:?usage: gen-hosts.sh <out.conf> <count> [mac-prefix]}"
prefix="${3:-aa:bb:cc}"

# Validate count is a non-negative integer.
case "$n" in
    ''|*[!0-9]*) echo "gen-hosts: count must be a positive integer" >&2; exit 2 ;;
esac

mkdir -p "$(dirname "$out")"
: > "$out"

i=0
while [ "$i" -lt "$n" ]; do
    v=$((i + 1))
    m1=$(( (v >> 16) & 0xFF ))
    m2=$(( (v >> 8)  & 0xFF ))
    m3=$((  v        & 0xFF ))
    third=$(( (i / 254) % 254 + 1 ))
    fourth=$(( i % 254 + 1 ))
    printf 'dhcp-host=%s:%02x:%02x:%02x,10.0.%d.%d,host%d\n' \
        "$prefix" "$m1" "$m2" "$m3" "$third" "$fourth" "$i" >> "$out"
    i=$((i + 1))
done

echo "gen-hosts: wrote $n dhcp-host lines into $out" >&2
