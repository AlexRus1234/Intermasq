// i18n API error: switching the UI to English and submitting a duplicate MAC
// surfaces a toast with the EN translation of the backend error code.
//
// Flow:
//   1. Seed aa:e2:...:01 via the API (so the MAC already exists).
//   2. In the UI, switch locale to EN via the user menu (🌐 English — the
//      menu labels the switch target, so it reads "English" while in RU).
//   3. Open HostForm, type the same MAC, Add → backend returns 409
//      mac_duplicate → HostForm.saveHost shows toast.error(translateApiError)
//      → en.json api.mac_duplicate = "This MAC is already used in another
//      file".
//
// We assert the toast body contains "already used" — an EN-only substring,
// which simultaneously proves the locale switched AND the API-error path is
// translated. The MAC input placeholder is hardcoded ("MAC (aa:bb...)"),
// not i18n, so it's a stable anchor.

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const MAC = 'aa:e2:00:00:00:01'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: MAC, ip: '10.99.20.1', hostname: 'i18n-one', file: `${CONF_DIR}/e2e-i18n.conf` },
  ])
})

test('i18n: duplicate MAC shows an EN toast after locale switch', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Switch locale to EN. While in RU (default) the menu item reads "🌐 English".
  await page.locator('.dropdown-toggle').click()
  await page.locator('.dropdown-item', { hasText: 'English' }).click()

  // Submit a duplicate MAC via the HostForm (static tab is the default).
  const form = page.locator('.card.p-3.shadow-sm')
  await form.locator('input[placeholder="MAC (aa:bb...)"]').fill(MAC)
  await form.locator('.input-group:has(.btn-success) input.form-control').fill(`${CONF_DIR}/e2e-i18n.conf`)
  await form.locator('.input-group .btn-success').click()

  // EN toast from translateApiError('mac_duplicate').
  await expect(page.locator('.toast-body', { hasText: /already used/i })).toBeVisible({ timeout: 10000 })
})
