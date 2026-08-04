# tests/suites/84-restart-self.sh — POST /api/restart-self.
# P3.8: this endpoint had no smoke coverage. In ci-mode (which smoke assumes —
# see smoke.sh usage example `-ci-mode=true`) it returns 200
# {"status":"restarting"} WITHOUT actually restarting: the RestartSelf
# goroutine is gated on `if !*CiMode` (main.go:264), so the server stays up
# and subsequent suites keep running. Against a NON-ci binary this endpoint
# WOULD restart the server — do not run this suite against such a binary.

if require_jwt "restart-self" 1; then
    S=$(POST "$JWT" "/api/restart-self" "{}")
    check "POST /api/restart-self → 200 (ci-mode no-op)" 200 "$S" || true
fi
