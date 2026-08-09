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
