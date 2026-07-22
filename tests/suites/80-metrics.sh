# tests/suites/80-metrics.sh — /metrics auth methods + A8 body-on-401.
# No JWT required for this suite.

section "metrics"

# A8: /metrics without auth returns 401 but with EMPTY body (cosmetic issue).
S=$(PGET "/metrics")
check "GET /metrics without auth → 401" 401 "$S" || true
METRICS_NOAUTH_BODY_SIZE=$(wc -c < /tmp/smoke.body)
echo "  body bytes on 401: $METRICS_NOAUTH_BODY_SIZE"
if [ "$METRICS_NOAUTH_BODY_SIZE" -gt 2 ]; then
    check "A8: 401 has body (currently empty)" 1 1 A8 || true
else
    check "A8: 401 has body (currently empty)" 1 0 A8 || true
fi

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer ${JWT:-invalid}" "$BASE/metrics")
check "GET /metrics with Bearer JWT" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "X-API-Key: $SECRET" "$BASE/metrics")
check "GET /metrics with X-API-Key" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$BASE/metrics?token=$SECRET")
check "GET /metrics with ?token=" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$BASE/metrics?token=wrong")
check "GET /metrics with wrong ?token= → 401" 401 "$S" || true

# Spot-check metric names
curl -s -H "X-API-Key: $SECRET" "$BASE/metrics" > /tmp/smoke.metrics
for m in intermasq_hosts_total intermasq_reloads_total intermasq_dnsmasq_active intermasq_uptime_seconds; do
    if grep -q "^$m " /tmp/smoke.metrics; then
        check "metric $m present" 0 0 || true
    else
        check "metric $m present" 0 1 || true
    fi
done
