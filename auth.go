package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var users = make(map[string]string)

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
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user, "exp": time.Now().Add(72 * time.Hour).Unix(),
	})
	s, _ := token.SignedString(SecretKey)
	return s
}

func authMiddleware(c *gin.Context) {
	// 1. ПРОВЕРКА STATIC API KEY (Для скриптов и плагинов)
	// Если пришел правильный X-API-Key, пропускаем без JWT
	apiKey := c.GetHeader("X-API-Key")
	if apiKey != "" && apiKey == string(SecretKey) {
		c.Next()
		return
	}

	// 2. ПРОВЕРКА JWT (Для браузера)
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		c.AbortWithStatus(401)
		return
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) { return SecretKey, nil })
	
	if token == nil || !token.Valid {
		c.AbortWithStatus(401)
		return
	}
	c.Next()
}
