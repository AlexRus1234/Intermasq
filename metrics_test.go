// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"intermask/internal/auth"
	"intermask/internal/initd"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// TestBoolToFloat covers both branches of the trivial helper.
func TestBoolToFloat(t *testing.T) {
	if got := boolToFloat(true); got != 1 {
		t.Errorf("boolToFloat(true) = %v, want 1", got)
	}
	if got := boolToFloat(false); got != 0 {
		t.Errorf("boolToFloat(false) = %v, want 0", got)
	}
}

// TestWriteSimpleMetric verifies HELP / TYPE / value lines.
func TestWriteSimpleMetric(t *testing.T) {
	var b strings.Builder
	writeSimpleMetric(&b, "foo_total", "help text", 42.5)
	out := b.String()
	if !strings.Contains(out, "# HELP foo_total help text") {
		t.Errorf("missing HELP line: %q", out)
	}
	if !strings.Contains(out, "# TYPE foo_total gauge") {
		t.Errorf("missing TYPE line: %q", out)
	}
	if !strings.Contains(out, "foo_total 42.5") {
		t.Errorf("missing value line: %q", out)
	}
}

// TestWriteLabeledMetric covers the labelled-metric formatter which was at
// 0%. Asserts labels get serialized as k="v" pairs.
func TestWriteLabeledMetric(t *testing.T) {
	var b strings.Builder
	writeLabeledMetric(&b, "domain_up", "help", map[string]string{"domain": "wiki.lan"}, 1)
	out := b.String()
	if !strings.Contains(out, `domain_up{domain="wiki.lan"} 1`) {
		t.Errorf("expected labeled metric line, got: %q", out)
	}
	if !strings.Contains(out, "# TYPE domain_up gauge") {
		t.Errorf("missing TYPE line: %q", out)
	}
}

// TestWriteLabeledMetric_MultipleLabels checks the label-join with a comma.
func TestWriteLabeledMetric_MultipleLabels(t *testing.T) {
	var b strings.Builder
	writeLabeledMetric(&b, "m", "h", map[string]string{"a": "1", "b": "2"}, 0)
	out := b.String()
	if !strings.Contains(out, `a="1"`) || !strings.Contains(out, `b="2"`) {
		t.Errorf("expected both labels present, got: %q", out)
	}
	if !strings.Contains(out, ",") {
		t.Errorf("expected comma between labels, got: %q", out)
	}
}

// signTestJWT returns a valid HMAC-SHA256 JWT signed with the given key so
// the metrics auth path can exercise its Bearer / ?token= success branch.
func signTestJWT(t *testing.T, key []byte) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "test",
	})
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return s
}

// newMetricsContext builds a gin test context with the given method/target
// and an optional auth-related header pre-set by the caller.
func newMetricsContext(method, target string) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, nil)
	return w, c
}

// TestCheckMetricsAuth_APIKey covers the X-API-Key success path.
func TestCheckMetricsAuth_APIKey(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	_, c := newMetricsContext("GET", "/metrics")
	c.Request.Header.Set("X-API-Key", "test-secret-key-32-bytes-long!!")
	if !checkMetricsAuth(c) {
		t.Error("expected true for valid X-API-Key")
	}
}

// TestCheckMetricsAuth_APIKeyWrong covers the X-API-Key mismatch fall-through.
func TestCheckMetricsAuth_APIKeyWrong(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	_, c := newMetricsContext("GET", "/metrics")
	c.Request.Header.Set("X-API-Key", "wrong")
	if checkMetricsAuth(c) {
		t.Error("expected false for wrong X-API-Key")
	}
}

// TestCheckMetricsAuth_TokenQuerySecret covers ?token=<secret> success.
func TestCheckMetricsAuth_TokenQuerySecret(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	_, c := newMetricsContext("GET", "/metrics?token=test-secret-key-32-bytes-long!!")
	if !checkMetricsAuth(c) {
		t.Error("expected true for ?token=<secret>")
	}
}

// TestCheckMetricsAuth_TokenQueryJWT covers ?token=<jwt> success.
func TestCheckMetricsAuth_TokenQueryJWT(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	jwtStr := signTestJWT(t, auth.SecretKey)
	_, c := newMetricsContext("GET", "/metrics?token="+jwtStr)
	if !checkMetricsAuth(c) {
		t.Error("expected true for ?token=<valid jwt>")
	}
}

// TestCheckMetricsAuth_TokenQueryInvalid covers the malformed-?token= path:
// not equal to SecretKey AND jwt.Parse returns no valid token.
func TestCheckMetricsAuth_TokenQueryInvalid(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	_, c := newMetricsContext("GET", "/metrics?token=garbage")
	if checkMetricsAuth(c) {
		t.Error("expected false for garbage ?token=")
	}
}

// TestCheckMetricsAuth_BearerValid covers the Authorization: Bearer <jwt>
// success path.
func TestCheckMetricsAuth_BearerValid(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	jwtStr := signTestJWT(t, auth.SecretKey)
	_, c := newMetricsContext("GET", "/metrics")
	c.Request.Header.Set("Authorization", "Bearer "+jwtStr)
	if !checkMetricsAuth(c) {
		t.Error("expected true for valid Bearer")
	}
}

// TestCheckMetricsAuth_BearerInvalid covers the Bearer-failure path
// (prefix matches, but token not valid).
func TestCheckMetricsAuth_BearerInvalid(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	_, c := newMetricsContext("GET", "/metrics")
	c.Request.Header.Set("Authorization", "Bearer not-a-real-token")
	if checkMetricsAuth(c) {
		t.Error("expected false for invalid Bearer")
	}
}

// TestCheckMetricsAuth_NoAuth covers the all-empty fall-through to false.
func TestCheckMetricsAuth_NoAuth(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	_, c := newMetricsContext("GET", "/metrics")
	if checkMetricsAuth(c) {
		t.Error("expected false with no auth material")
	}
}

// TestMetricsHandler_AllMetricNames exercises the whole metricsHandler end
// to end (P2.7). The existing tests cover the formatters and the auth gate
// in isolation, but never the assembled exposition output — so renaming or
// dropping any of the 9 canonical metric names would not be caught. The two
// intermasq_domain_* labeled series are only emitted inside the dnsHealth
// loop (metrics.go:80-87), so we seed one entry to guarantee they appear.
func TestMetricsHandler_AllMetricNames(t *testing.T) {
	auth.SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))

	// metricsHandler -> checkDnsmasqStatus -> initd.Current().IsActive.
	// Without setupServer the package-level sysCaller is a nil interface and
	// the handler nil-derefs. NoneCaller is side-effect-free and returns
	// false for IsActive, which is all we need to exercise the output
	// assembly.
	initd.SetCurrentForTest(t, &initd.NoneCaller{})

	dnsHealthMu.Lock()
	dnsHealth["test.example.lan"] = dnsHealthEntry{Up: true, Latency: time.Millisecond}
	dnsHealthMu.Unlock()
	t.Cleanup(func() {
		dnsHealthMu.Lock()
		delete(dnsHealth, "test.example.lan")
		dnsHealthMu.Unlock()
	})

	jwtStr := signTestJWT(t, auth.SecretKey)
	w, c := newMetricsContext("GET", "/metrics")
	c.Request.Header.Set("Authorization", "Bearer "+jwtStr)

	metricsHandler(c)

	if w.Code != 200 {
		t.Fatalf("expected 200 from metricsHandler, got %d (body=%s)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	required := []string{
		"intermasq_hosts_total",
		"intermasq_leases_active",
		"intermasq_arp_online_total",
		"intermasq_dnsmasq_active",
		"intermasq_reloads_total",
		"intermasq_dnsmasq_test_failures_total",
		"intermasq_uptime_seconds",
		"intermasq_domain_up",
		"intermasq_domain_resolve_seconds",
	}
	for _, name := range required {
		if !strings.Contains(body, name) {
			t.Errorf("metricsHandler: missing metric %q in body\n--- body ---\n%s", name, body)
		}
	}
}
