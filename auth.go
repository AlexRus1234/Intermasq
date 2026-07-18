// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var users = make(map[string]string)

var (
	blacklist   = make(map[string]time.Time)
	blacklistMu sync.RWMutex
)

func init() {
	go cleanBlacklistLoop()
}

func cleanBlacklistLoop() {
	for {
		time.Sleep(10 * time.Minute)
		blacklistMu.Lock()
		now := time.Now()
		for id, exp := range blacklist {
			if exp.Before(now) {
				delete(blacklist, id)
			}
		}
		blacklistMu.Unlock()
	}
}

func revokeToken(jti string, exp time.Time) {
	blacklistMu.Lock()
	blacklist[jti] = exp
	blacklistMu.Unlock()
}

func isTokenRevoked(jti string) bool {
	blacklistMu.RLock()
	defer blacklistMu.RUnlock()
	_, ok := blacklist[jti]
	return ok
}

var (
	rateLimitStore = make(map[string][]time.Time)
	rateLimitMu    sync.Mutex
	rateLimitClean = time.Now()
)

func rateLimitMiddleware(maxAttempts int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		rateLimitMu.Lock()
		if time.Since(rateLimitClean) > 5*time.Minute {
			for ip, stamps := range rateLimitStore {
				cutoff := time.Now().Add(-window)
				var kept []time.Time
				for _, s := range stamps {
					if s.After(cutoff) {
						kept = append(kept, s)
					}
				}
				if len(kept) == 0 {
					delete(rateLimitStore, ip)
				} else {
					rateLimitStore[ip] = kept
				}
			}
			rateLimitClean = time.Now()
		}
		rateLimitMu.Unlock()

		ip := c.ClientIP()
		rateLimitMu.Lock()
		stamps := rateLimitStore[ip]
		cutoff := time.Now().Add(-window)
		var recent []time.Time
		for _, s := range stamps {
			if s.After(cutoff) {
				recent = append(recent, s)
			}
		}
		recent = append(recent, time.Now())
		rateLimitStore[ip] = recent
		rateLimitMu.Unlock()

		if len(recent) > maxAttempts {
			c.AbortWithStatusJSON(429, gin.H{"error": "too_many_attempts"})
			return
		}
		c.Next()
	}
}

func loadUsers() {
	if _, err := os.Stat(*DBPath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(*DBPath), 0700)
		return
	}
	data, _ := os.ReadFile(*DBPath)
	json.Unmarshal(data, &users)
}

func saveUsers() error {
	data, _ := json.MarshalIndent(users, "", "  ")
	return os.WriteFile(*DBPath, data, 0600)
}

func makeToken(user string) string {
	jti := fmt.Sprintf("%s-%d", user, time.Now().UnixNano())
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user, "exp": time.Now().Add(72 * time.Hour).Unix(),
		"jti": jti,
	})
	s, _ := token.SignedString(SecretKey)
	return s
}

func authMiddleware(c *gin.Context) {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey != "" && apiKey == string(SecretKey) {
		c.Set("user", "api-key")
		c.Next()
		return
	}

	authHeader := c.GetHeader("Authorization")
	var tokenStr string
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		// NOTE: ?token= query fallback was removed because it leaked JWTs
		// into access logs and referrer headers (SSE used it via EventSource).
		// /metrics still accepts ?token= because Prometheus scrape configs
		// cannot easily send custom headers — see checkMetricsAuth.
		c.AbortWithStatus(401)
		return
	}
	token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) { return SecretKey, nil })

	if token == nil || !token.Valid {
		c.AbortWithStatus(401)
		return
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if jti, ok := claims["jti"].(string); ok && isTokenRevoked(jti) {
			c.AbortWithStatus(401)
			return
		}
		if sub, ok := claims["sub"].(string); ok {
			c.Set("user", sub)
		}
	}
	c.Next()
}
