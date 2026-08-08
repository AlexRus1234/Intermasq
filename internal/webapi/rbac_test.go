package webapi

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"intermask/internal/auth"
	"intermask/internal/models"
)

func TestAdminMiddlewareRejectsRegularUser(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", models.RoleUser)
	auth.AdminMiddleware(c)
	if w.Code != 403 {
		t.Fatalf("expected regular user to receive 403, got %d", w.Code)
	}
}

func TestAdminMiddlewareAllowsAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", models.RoleAdmin)
	auth.AdminMiddleware(c)
	if w.Code != 200 {
		t.Fatalf("expected admin middleware to continue, got %d", w.Code)
	}
}
