# tests/suites/80-metrics.sh — /metrics auth methods + A8 body-on-401.
# No JWT required for this suite.

section "metrics"

# A8 was: /metrics returned 401 with an EMPTY body. FIXED in internal/metrics/metrics.go:59
# (AbortWithStatusJSON now writes {"error":"auth_required"}). A8 is no longer
# in known-bugs.txt, so this is an HONEST regression: the 401 body must be
# non-empty AND contain "auth_required". (Previously this check's description
# said "currently empty" while its PASS branch required non-empty — an
# inversion that would mislead anyone "fixing" the comparison.)
S=$(PGET "/metrics")
check "GET /metrics without auth → 401" 401 "$S" || true
METRICS_NOAUTH_BODY_SIZE=$(wc -c < /tmp/smoke.body)
echo "  body bytes on 401: $METRICS_NOAUTH_BODY_SIZE"
if [ "$METRICS_NOAUTH_BODY_SIZE" -gt 2 ] && grep -q 'auth_required' /tmp/smoke.body; then
    check "A8: 401 body non-empty AND contains auth_required" 1 1
else
    check "A8: 401 body non-empty AND contains auth_required" 1 0
fi

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer ${JWT:-invalid}" "$BASE/metrics")
check "GET /metrics with Bearer JWT" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "X-API-Key: $SECRET" "$BASE/metrics")
check "GET /metrics with X-API-Key" 200 "$S" || true

S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" "$BASE/metrics?token=$SECRET")
check "GET /metrics with ?token= rejected" 401 "$S" || true

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
