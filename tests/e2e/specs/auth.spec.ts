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

// Auth flow against a FRESH browser context (no seeded token).
//
// globalSetup already created the admin, so the app lands on the login
// screen. We exercise the real UI: fill credentials, submit, land in the
// dashboard, then log out and confirm we're back at the login screen with
// the token purged and subsequent API calls rejected (401).

import { test, expect } from '@playwright/test'

// Opt out of the shared storageState so this spec starts unauthenticated.
test.use({ storageState: { cookies: [], origins: [] } })

test('login then logout via UI', async ({ page }) => {
  await page.goto('/')

  const userInput = page.locator('input.form-control').nth(0)
  const passInput = page.locator('input.form-control').nth(1)
  await userInput.fill('admin')
  await passInput.fill('pass1234')
  await page.locator('.btn-primary').click()

  // Dashboard indicator: the user-menu toggle is rendered only when
  // store.token is set (App.vue `v-if="store.token"`).
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Logout via the menu's single .text-danger item.
  await page.locator('.dropdown-toggle').click()
  await page.locator('.dropdown-item.text-danger').click()

  // Back to the auth screen.
  await expect(page.locator('.btn-primary')).toBeVisible({ timeout: 10000 })

  // Strong asserts (mutation-pass Блок C showed the .btn-primary check alone
  // is weak): the token must be gone from localStorage (store.js logout()
  // calls localStorage.removeItem('token')), and an unauthenticated API call
  // must be rejected with 401 (JWT, no refresh token).
  const token = await page.evaluate(() => localStorage.getItem('token'))
  expect(token).toBeNull()

  const status = await page.evaluate(() => fetch('/api/hosts').then((r) => r.status))
  expect(status).toBe(401)
})
