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

// RBAC: a non-admin user must not see admin-only controls.
//
// The backend splits /api routes into `auth` (any authenticated user) and
// `admin` (AdminMiddleware). The frontend now decodes the JWT `role` claim
// (store.isAdmin) and gates the admin-only affordances with v-if. This spec
// is the regression guard: create a default-role (non-admin) user, log in as
// it, and assert every admin-only control is absent from the DOM while an
// auth-any-user control (Backup download) is still present — proving the
// page rendered normally and the hiding is role-driven, not a broken page.
//
// Admin-only controls gated by store.isAdmin (mirrors register.go admin.*):
//   - Apply  (POST /api/reload)             App.vue
//   - Restore (POST /api/backup/restore)    App.vue + SafetyTab.vue
//   - Restart (POST /api/restart-self)      App.vue menu
//   - Users tab (GET/POST/DELETE /api/users) App.vue
//   - Rollback (POST /api/rollback)         Static/Dns/Config views
//   - Raw editor toggle (PUT /api/files)    DnsmasqConfig.vue
//   - History restore (POST /api/history/restore) HistoryModal.vue
// This spec covers the dashboard-level controls (Apply/Restore/Users/Restart);
// the per-view buttons are checked by code review + the single isAdmin getter
// that powers them all.

import { test, expect, request } from '@playwright/test'
import { apiLogin, BASE_URL } from '../lib/api'

const VIEWER_USER = 'viewer'
const VIEWER_PASS = 'view1234'

test('rbac: non-admin user does not see admin-only controls', async ({ browser }) => {
  // --- seed: admin creates a default-role user (POST /api/users assigns
  // RoleUser unless an explicit admin path is taken). Ignore 409 on re-runs
  // (user_exists) — the login below works either way.
  const adminToken = await apiLogin()
  const seedCtx = await request.newContext({ baseURL: BASE_URL })
  await seedCtx.post('/api/users', {
    data: { username: VIEWER_USER, password: VIEWER_PASS },
    headers: { Authorization: `Bearer ${adminToken}` },
  })

  // Log in as the non-admin user. /api/login is rate-limited (10/min) but a
  // successful login resets the counter (auth.go), and the global setup +
  // this single login stay well under the cap (workers:1, sequential).
  const loginRes = await seedCtx.post('/api/login', {
    data: { username: VIEWER_USER, password: VIEWER_PASS },
  })
  expect(loginRes.ok(), `viewer login failed: ${loginRes.status()} ${await loginRes.text()}`).toBeTruthy()
  const { token: viewerToken } = await loginRes.json()
  expect(viewerToken).toBeTruthy()
  await seedCtx.dispose()

  // Fresh context with ONLY the viewer token — do not inherit the admin
  // storageState that globalSetup wrote for the rest of the suite.
  const viewerContext = await browser.newContext({
    storageState: {
      cookies: [],
      origins: [
        {
          origin: new URL(BASE_URL).origin,
          localStorage: [{ name: 'token', value: viewerToken }],
        },
      ],
    },
  })
  const page = await viewerContext.newPage()

  await page.goto('/')
  // Wait for the dashboard toolbar (Backup download is auth-any-user, so it
  // serves as both the readiness gate and the positive control).
  const backupBtn = page.locator('button.btn-outline-info', { hasText: '💾' })
  await expect(backupBtn).toBeVisible({ timeout: 15000 })

  // --- positive: auth-any-user controls ARE rendered for a non-admin ---
  await expect(backupBtn).toBeVisible()           // GET /api/backup
  await expect(page.locator('.btn-group button', { hasText: '⚙️' })).toBeVisible() // config tab (GET /api/config)

  // --- negative: admin-only controls are ABSENT from the DOM (v-if false) ---
  await expect(page.locator('button.btn-warning', { hasText: '🔄' })).toHaveCount(0)              // Apply (/reload)
  await expect(page.locator('button.btn-outline-warning', { hasText: '📤' })).toHaveCount(0)       // Restore (/backup/restore)
  await expect(page.locator('.btn-group button', { hasText: '👥' })).toHaveCount(0)               // Users tab (/users)

  // Restart lives in the dropdown menu; v-if removes the <li> entirely for
  // non-admins, so the item count is 0 without opening the dropdown.
  await expect(page.locator('.dropdown-item', { hasText: '🔄' })).toHaveCount(0)                   // Restart (/restart-self)

  await viewerContext.close()
})
