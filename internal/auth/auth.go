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

package auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"intermask/internal/models"
)

var (
	DBPath    = flag.String("db", "/etc/intermasq/users.json", "Path to user database")
	SecretKey = []byte(os.Getenv("INTERMASQ_SECRET"))
	users     = make(map[string]string)
	userRoles = make(map[string]string)
	usersMu   sync.RWMutex
)

var ErrUserExists = errors.New("user already exists")

var (
	blacklist   = make(map[string]time.Time)
	blacklistMu sync.RWMutex
)

const maxStoreSize = 100000

var (
	userTokenVersions = make(map[string]uint64)
	userTokenMu       sync.RWMutex
)

func init() { go cleanBlacklistLoop() }

func cleanBlacklistLoop() {
	for {
		time.Sleep(10 * time.Minute)
		cleanupBlacklistOnce(time.Now())
	}
}

func cleanupBlacklistOnce(now time.Time) {
	blacklistMu.Lock()
	defer blacklistMu.Unlock()
	for id, exp := range blacklist {
		if exp.Before(now) {
			delete(blacklist, id)
		}
	}
}

func RevokeToken(jti string, exp time.Time) {
	blacklistMu.Lock()
	if _, exists := blacklist[jti]; !exists && len(blacklist) >= maxStoreSize {
		var oldestID string
		var oldest time.Time
		for id, expiry := range blacklist {
			if oldestID == "" || expiry.Before(oldest) {
				oldestID = id
				oldest = expiry
			}
		}
		delete(blacklist, oldestID)
	}
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

func RateLimitMiddleware(maxAttempts int, window time.Duration) gin.HandlerFunc {
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

		ip := c.RemoteIP()
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
		if len(stamps) == 0 && len(rateLimitStore) >= maxStoreSize {
			var oldestIP string
			var oldest time.Time
			for candidateIP, candidateStamps := range rateLimitStore {
				if len(candidateStamps) == 0 {
					oldestIP = candidateIP
					break
				}
				candidateOldest := candidateStamps[0]
				if oldestIP == "" || candidateOldest.Before(oldest) {
					oldestIP = candidateIP
					oldest = candidateOldest
				}
			}
			delete(rateLimitStore, oldestIP)
		}
		rateLimitStore[ip] = recent
		rateLimitMu.Unlock()

		if len(recent) > maxAttempts {
			c.AbortWithStatusJSON(429, gin.H{"error": "too_many_attempts"})
			return
		}
		c.Next()
	}
}

func RateLimitReset(ip string) {
	rateLimitMu.Lock()
	delete(rateLimitStore, ip)
	rateLimitMu.Unlock()
}

func LoadUsers() {
	if _, err := os.Stat(*DBPath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(*DBPath), 0700)
		return
	}
	data, err := os.ReadFile(*DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Cannot read user database %s: %v\n", *DBPath, err)
		os.Exit(1)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(data, &stored); err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Cannot parse user database %s: %v\n", *DBPath, err)
		os.Exit(1)
	}
	loadedUsers := make(map[string]string, len(stored))
	loadedRoles := make(map[string]string, len(stored))
	for name, raw := range stored {
		var hash string
		if err := json.Unmarshal(raw, &hash); err == nil {
			loadedUsers[name] = hash
			loadedRoles[name] = models.RoleAdmin
			continue
		}
		var user models.User
		if err := json.Unmarshal(raw, &user); err != nil || user.PasswordHash == "" {
			fmt.Fprintf(os.Stderr, "[FATAL] Cannot parse user database %s: invalid user %q\n", *DBPath, name)
			os.Exit(1)
		}
		loadedUsers[name] = user.PasswordHash
		if user.Role == "" {
			user.Role = models.RoleAdmin
		}
		loadedRoles[name] = user.Role
	}
	usersMu.Lock()
	users = loadedUsers
	userRoles = loadedRoles
	usersMu.Unlock()
}

func SaveUsers() error {
	usersMu.RLock()
	defer usersMu.RUnlock()
	return saveUsersLocked()
}

func saveUsersLocked() error {
	allAdmins := true
	stored := make(map[string]models.User, len(users))
	for name, hash := range users {
		role := userRoles[name]
		if role == "" {
			role = models.RoleAdmin
		}
		stored[name] = models.User{Username: name, PasswordHash: hash, Role: role}
		if role != models.RoleAdmin {
			allAdmins = false
		}
	}
	var data []byte
	var err error
	if allAdmins {
		legacy := make(map[string]string, len(stored))
		for name, user := range stored {
			legacy[name] = user.PasswordHash
		}
		data, err = json.MarshalIndent(legacy, "", "  ")
	} else {
		data, err = json.MarshalIndent(stored, "", "  ")
	}
	if err != nil {
		return err
	}
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

func UserCount() int {
	usersMu.RLock()
	defer usersMu.RUnlock()
	return len(users)
}

func UserNames() []string {
	usersMu.RLock()
	defer usersMu.RUnlock()
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	return names
}

func GetUser(name string) (string, bool) {
	usersMu.RLock()
	defer usersMu.RUnlock()
	hash, ok := users[name]
	return hash, ok
}

func HasUser(name string) bool {
	_, ok := GetUser(name)
	return ok
}

func AddUser(name, hash string) error {
	usersMu.Lock()
	defer usersMu.Unlock()
	if _, exists := users[name]; exists {
		return ErrUserExists
	}
	users[name] = hash
	role := models.RoleUser
	if len(users) == 1 {
		role = models.RoleAdmin
	}
	userRoles[name] = role
	if err := saveUsersLocked(); err != nil {
		delete(users, name)
		delete(userRoles, name)
		return err
	}
	return nil
}

func UpdateUser(name, hash string) error {
	usersMu.Lock()
	defer usersMu.Unlock()
	users[name] = hash
	if userRoles[name] == "" {
		userRoles[name] = models.RoleAdmin
	}
	if err := saveUsersLocked(); err != nil {
		return err
	}
	revokeUserTokens(name)
	return nil
}

func DeleteUser(name string) error {
	usersMu.Lock()
	defer usersMu.Unlock()
	delete(users, name)
	delete(userRoles, name)
	if err := saveUsersLocked(); err != nil {
		return err
	}
	revokeUserTokens(name)
	return nil
}

func SetUser(name, hash string) {
	usersMu.Lock()
	users[name] = hash
	if userRoles[name] == "" {
		userRoles[name] = models.RoleAdmin
	}
	usersMu.Unlock()
}

func ClearUsers() {
	usersMu.Lock()
	users = make(map[string]string)
	userRoles = make(map[string]string)
	usersMu.Unlock()
}

func UserRole(name string) string {
	usersMu.RLock()
	defer usersMu.RUnlock()
	if _, ok := users[name]; !ok {
		return models.RoleAdmin
	}
	role := userRoles[name]
	if role == "" {
		return models.RoleAdmin
	}
	return role
}

func MakeToken(user string) string {
	jti := fmt.Sprintf("%s-%d", user, time.Now().UnixNano())
	userTokenMu.RLock()
	version := userTokenVersions[user]
	userTokenMu.RUnlock()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user, "exp": time.Now().Add(72 * time.Hour).Unix(), "jti": jti, "ver": version, "role": UserRole(user),
	})
	s, _ := token.SignedString(SecretKey)
	return s
}

func Middleware(c *gin.Context) {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey != "" && subtle.ConstantTimeCompare([]byte(apiKey), SecretKey) == 1 {
		c.Set("user", "api-key")
		c.Set("role", models.RoleAdmin)
		c.Next()
		return
	}
	authHeader := c.GetHeader("Authorization")
	var tokenStr string
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.AbortWithStatus(401)
		return
	}
	token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return SecretKey, nil
	})
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
			userTokenMu.RLock()
			currentVersion := userTokenVersions[sub]
			userTokenMu.RUnlock()
			version, ok := claims["ver"].(float64)
			if !ok || version < 0 || uint64(version) != currentVersion {
				c.AbortWithStatus(401)
				return
			}
			c.Set("user", sub)
			role, _ := claims["role"].(string)
			if role == "" {
				role = UserRole(sub)
			}
			c.Set("role", role)
		}
	}
	c.Next()
}

func AdminMiddleware(c *gin.Context) {
	role, _ := c.Get("role")
	if role != models.RoleAdmin {
		c.AbortWithStatusJSON(403, gin.H{"error": "admin only"})
		return
	}
	c.Next()
}

func revokeUserTokens(name string) {
	userTokenMu.Lock()
	userTokenVersions[name]++
	userTokenMu.Unlock()
}

// SetSecretForTest ... Exported for cross-package tests during modularization.
func SetSecretForTest(t *testing.T, secret []byte) {
	t.Helper()
	original := SecretKey
	SecretKey = secret
	t.Cleanup(func() { SecretKey = original })
}
