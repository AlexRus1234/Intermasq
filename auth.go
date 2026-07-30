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

// usersMu guards the in-memory `users` map against concurrent
// read-modify-write sequences. Go's runtime panics if a map is read while
// another goroutine is writing to it, so even len(users) needs a lock.
// Held across read→modify→saveUsers() in handlers_users.go so two
// concurrent POST /api/users cannot interleave and silently drop one of
// the new records.
var usersMu sync.RWMutex

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
		cleanupBlacklistOnce(time.Now())
	}
}

// cleanupBlacklistOnce performs a single sweep of the revocation blacklist:
// removes any entry whose expiry is in the past. Extracted from
// cleanBlacklistLoop so the per-iteration logic is unit-testable without
// sleeping or spawning the background goroutine. The caller is responsible
// for any locking around the blacklist.
func cleanupBlacklistOnce(now time.Time) {
	blacklistMu.Lock()
	defer blacklistMu.Unlock()
	for id, exp := range blacklist {
		if exp.Before(now) {
			delete(blacklist, id)
		}
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

// rateLimitReset clears the rate-limit counter for a single IP. Called
// from loginHandler after a successful login so a legitimate user who
// mistyped their password a few times is not left counting against the
// cap. Brute-force protection is unchanged for failed attempts — they
// keep accumulating until the window expires.
func rateLimitReset(ip string) {
	rateLimitMu.Lock()
	delete(rateLimitStore, ip)
	rateLimitMu.Unlock()
}

func loadUsers() {
	if _, err := os.Stat(*DBPath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(*DBPath), 0700)
		return
	}
	data, err := os.ReadFile(*DBPath)
	if err != nil {
		// Не区别 «нет прав» от «файл пропал между Stat и Read» — любое
		// повреждение базы пользователей должно блокировать старт, иначе
		// authMiddleware увидит пустую map и любой POST /api/setup создаст
		// нового admin'а (см. statusHandler → setup_required).
		fmt.Fprintf(os.Stderr, "[FATAL] Cannot read user database %s: %v\n", *DBPath, err)
		os.Exit(1)
	}
	if err := json.Unmarshal(data, &users); err != nil {
		// Повреждённый JSON — та же логика: лучше упасть, чем тихо
		// обнулить базу и открыть /api/setup.
		fmt.Fprintf(os.Stderr, "[FATAL] Cannot parse user database %s: %v\n", *DBPath, err)
		os.Exit(1)
	}
}

// saveUsers writes the user database atomically: tmp file in the same
// directory + rename.Crash mid-write leaves the previous file intact.
func saveUsers() error {
	data, _ := json.MarshalIndent(users, "", "  ")
	dir := filepath.Dir(*DBPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := *DBPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, *DBPath)
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
