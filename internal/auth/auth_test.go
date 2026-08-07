// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package auth

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func resetAuthState() {
	usersMu.Lock()
	users = make(map[string]string)
	usersMu.Unlock()
	blacklistMu.Lock()
	blacklist = make(map[string]time.Time)
	blacklistMu.Unlock()
	rateLimitMu.Lock()
	rateLimitStore = make(map[string][]time.Time)
	rateLimitMu.Unlock()
}

func TestLoadUsersMissingFileIsOK(t *testing.T) {
	resetAuthState()
	*DBPath = filepath.Join(t.TempDir(), "absent.json")
	LoadUsers()
	if UserCount() != 0 {
		t.Fatalf("expected empty users, got %d", UserCount())
	}
}

func TestSaveUsersAtomic(t *testing.T) {
	resetAuthState()
	*DBPath = filepath.Join(t.TempDir(), "users.json")
	SetUser("admin", "hash")
	SetUser("bob", "hash2")
	if err := SaveUsers(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(*DBPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil || len(got) != 2 {
		t.Fatalf("invalid users database: %v", err)
	}
	if _, err := os.Stat(*DBPath + ".tmp"); !os.IsNotExist(err) {
		t.Error("temporary file remains after atomic save")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))
	token := MakeToken("admin")
	parsed, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) { return SecretKey, nil })
	if err != nil || !parsed.Valid {
		t.Fatalf("token is invalid: %v", err)
	}
}

func TestTokenRevoked(t *testing.T) {
	resetAuthState()
	RevokeToken("jti", time.Now().Add(time.Hour))
	if !isTokenRevoked("jti") {
		t.Fatal("token should be revoked")
	}
}

func TestCleanBlacklist(t *testing.T) {
	resetAuthState()
	RevokeToken("expired", time.Now().Add(-time.Hour))
	RevokeToken("fresh", time.Now().Add(time.Hour))
	cleanupBlacklistOnce(time.Now())
	if isTokenRevoked("expired") || !isTokenRevoked("fresh") {
		t.Fatal("blacklist cleanup removed the wrong entries")
	}
}

func TestCleanupBlacklistOnce_EmptyMap(t *testing.T) {
	resetAuthState()
	cleanupBlacklistOnce(time.Now())
	if len(blacklist) != 0 {
		t.Fatal("empty blacklist changed")
	}
}

func TestRateLimitUnderLimit(t *testing.T) {
	resetAuthState()
	handler := RateLimitMiddleware(2, time.Minute)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.1:1234"
		handler(c)
		if w.Code == 429 {
			t.Fatal("request was limited too early")
		}
	}
}

func TestRateLimitOverLimit(t *testing.T) {
	resetAuthState()
	handler := RateLimitMiddleware(1, time.Minute)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = "10.0.0.2:1234"
		handler(c)
		if i == 1 && w.Code != 429 {
			t.Fatal("second request was not limited")
		}
	}
}

func TestRateLimitReset(t *testing.T) {
	resetAuthState()
	RateLimitReset("10.0.0.1")
	rateLimitStore["10.0.0.1"] = []time.Time{time.Now()}
	RateLimitReset("10.0.0.1")
	if _, ok := rateLimitStore["10.0.0.1"]; ok {
		t.Fatal("rate-limit entry remains after reset")
	}
}

func TestAuthMiddlewareBearerHeader(t *testing.T) {
	resetAuthState()
	SetSecretForTest(t, []byte("test-secret-key-32-bytes-long!!"))
	token := MakeToken("admin")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	Middleware(c)
	if w.Code == 401 || c.GetString("user") != "admin" {
		t.Fatal("bearer token was rejected")
	}
}

func TestAuthMiddlewareNoCredentials(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	Middleware(c)
	if w.Code != 401 {
		t.Fatal("missing credentials were accepted")
	}
}
