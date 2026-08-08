package webapi

// User management handlers: create, delete, list users, change own
// password, logout (JWT revocation). All mutations are guarded by the
// process-wide mutex `mu` AND a dedicated usersMu (see auth.go) — the
// latter is held across read-modify-write so concurrent POST /api/users
// cannot interleave and lose a record.

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"intermask/internal/audit"
	"intermask/internal/auth"
	"intermask/internal/models"
)

func getUsersHandler(c *gin.Context) {
	c.JSON(200, gin.H{"users": auth.UserNames()})
}

func createUserHandler(c *gin.Context) {
	var req models.AuthReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "missing_fields"})
		return
	}
	if len(req.Username) > 64 {
		c.JSON(400, gin.H{"error": "username_too_long"})
		return
	}
	if auth.HasUser(req.Username) {
		c.JSON(409, gin.H{"error": "user_exists"})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err := auth.AddUser(req.Username, string(hash)); err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			c.JSON(409, gin.H{"error": "user_exists"})
			return
		}
		c.JSON(500, gin.H{"error": "save_error"})
		return
	}
	audit.WriteAudit(audit.AuditEntry{
		User:   getUser(c),
		Action: "user_create",
		Mac:    req.Username,
	})
	c.JSON(200, gin.H{"status": "ok"})
}

func deleteUserHandler(c *gin.Context) {
	name := c.Param("name")
	currentUser := getUser(c)
	if name == currentUser {
		c.JSON(400, gin.H{"error": "cannot_delete_self"})
		return
	}
	if !auth.HasUser(name) {
		c.JSON(404, gin.H{"error": "user_not_found"})
		return
	}
	if err := auth.DeleteUser(name); err != nil {
		c.JSON(500, gin.H{"error": "save_error"})
		return
	}
	audit.WriteAudit(audit.AuditEntry{
		User:   currentUser,
		Action: "user_delete",
		Mac:    name,
	})
	c.JSON(200, gin.H{"status": "deleted"})
}

func changePasswordHandler(c *gin.Context) {
	var req models.UserPasswordReq
	if err := c.BindJSON(&req); err != nil {
		return
	}
	if req.NewPassword == "" {
		c.JSON(400, gin.H{"error": "missing_fields"})
		return
	}
	currentUser := getUser(c)
	hash, ok := auth.GetUser(currentUser)
	if !ok || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		c.JSON(401, gin.H{"error": "invalid_credentials"})
		return
	}
	newHash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err := auth.UpdateUser(currentUser, string(newHash)); err != nil {
		c.JSON(500, gin.H{"error": "save_error"})
		return
	}
	audit.WriteAudit(audit.AuditEntry{
		User:   currentUser,
		Action: "password_change",
	})
	c.JSON(200, gin.H{"status": "ok"})
}

// logoutHandler revokes the caller's JWT by adding its jti to the
// in-memory blacklist. The blacklist is wiped on process restart — this
// is a deliberate simplification, see docs/new-features.md §8.
func logoutHandler(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) { return auth.SecretKey, nil })
		if token != nil {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if jti, ok := claims["jti"].(string); ok {
					if exp, ok := claims["exp"].(float64); ok {
						auth.RevokeToken(jti, time.Unix(int64(exp), 0))
					}
				}
			}
		}
	}
	c.JSON(200, gin.H{"status": "logged_out"})
}
