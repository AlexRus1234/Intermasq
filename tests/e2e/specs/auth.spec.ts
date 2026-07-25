// Auth flow against a FRESH browser context (no seeded token).
//
// globalSetup already created the admin, so the app lands on the login
// screen. We exercise the real UI: fill credentials, submit, land in the
// dashboard, then log out and confirm we're back at the login screen.

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
})
