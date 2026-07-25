// Config editor file lifecycle via the UI:
//   open config tab → "+ new file" → type name → create → tab appears →
//   select it → 🗑 delete → tab disappears.
//
// Deliberately covers only create + delete of a file (POST/DELETE
// /api/config/file), which do NOT run `dnsmasq --test`. The visual
// "save" path goes through writeFileRaw → `dnsmasq --test` (known bug
// A13: tests the default config, not the file) and is intentionally left
// to a separate spec once that interaction is stable.

import { test, expect } from '@playwright/test'

const FILENAME = 'e2e-config.conf'

test('config file create via UI appears as tab, then deletes', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Config tab: the btn-group button carrying ⚙️ (scoped to btn-group so
  // the dropdown-toggle ⚙️ menu button can't match).
  await page.locator('.btn-group button', { hasText: '⚙️' }).click()

  // "+ new file" is the only .text-success nav-link in the config tab.
  await page.locator('.nav-link.text-success').click()

  const nameInput = page.locator('input[placeholder="filename.conf"]')
  await expect(nameInput).toBeVisible({ timeout: 5000 })
  await nameInput.fill(FILENAME)

  // ＋ create button lives in the .border-success card.
  await page.locator('.card.border-success .btn-success').click()

  // The new file shows up as a nav tab.
  const tab = page.locator('.nav-link', { hasText: FILENAME })
  await expect(tab).toBeVisible({ timeout: 10000 })

  // Select it so the per-file delete button renders, then delete.
  await tab.click()
  page.on('dialog', (d) => d.accept()) // deleteFile() uses confirm()
  await page.locator('.d-flex.gap-2 .btn-outline-danger').click()

  await expect(tab).toBeHidden({ timeout: 10000 })
})
