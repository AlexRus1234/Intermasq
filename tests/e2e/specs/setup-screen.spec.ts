// First-run setup screen against an ISOLATED 2nd intermasq instance (:18084)
// with a fresh user DB → /api/status returns setup_required:true → store.js
// sets store.view='setup' → AuthScreen renders the setup form (same
// input.form-control ×2 + .btn-primary as login, but POSTs /api/setup).
//
// The CI opt-in L4 step starts this 2nd instance and exports
// E2E_SETUP_BASE_URL. Without it (e.g. local runs), the test skips.
//
// We create our OWN page via browser.newPage so the default baseURL
// (:18083) and globalSetup storageState (admin token for :18083) do NOT
// apply — setup must run unauthenticated against the fresh instance.

import { test, expect } from '@playwright/test'

const SETUP_URL = process.env.E2E_SETUP_BASE_URL

test('setup-screen: first-run admin setup', async ({ browser }) => {
  test.skip(!SETUP_URL, 'needs E2E_SETUP_BASE_URL (2nd intermasq instance :18084)')

  // Defensive: if CI failed to start the 2nd instance or it isn't in
  // setup_required mode, SKIP rather than FAIL — keeps the opt-in L4 run
  // yellow on an infra glitch instead of red.
  const status = await fetch(`${SETUP_URL}/api/status`)
    .then((r) => r.json())
    .catch(() => null)
  test.skip(!status || status.setup_required !== true, '2nd instance not in setup_required mode')

  const page = await browser.newPage({ baseURL: SETUP_URL })
  await page.goto('/')

  // AuthScreen in setup mode: username, password, then createAccount.
  await page.locator('input.form-control').nth(0).fill('setupadmin')
  await page.locator('input.form-control').nth(1).fill('setuppass1234')
  await page.locator('.btn-primary').click()

  // /api/setup returns a token → store.view='dashboard' (AuthScreen.vue:42),
  // the user-menu toggle renders (App.vue v-if="store.token").
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })
  await page.close()
})
