// Locale toggle: clicking the 🌐 menu item flips the locale and persists it.
//
// We deliberately do NOT depend on which locale is the default. The
// invariant is: after the click, localStorage.locale is non-empty and the
// 🌐 item's label has changed (English ↔ Русский).

import { test, expect } from '@playwright/test'

test('locale toggle changes menu label and persists', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await page.locator('.dropdown-toggle').click()
  const localeItem = page.locator('.dropdown-item', { hasText: '🌐' })
  const before = (await localeItem.innerText()).trim()

  await localeItem.click()

  const stored = await page.evaluate(() => localStorage.getItem('locale'))
  expect(stored).toBeTruthy()

  // Reopen the menu and read the label again — it must have flipped.
  await page.locator('.dropdown-toggle').click()
  const after = (
    await page.locator('.dropdown-item', { hasText: '🌐' }).innerText()
  ).trim()
  expect(before).not.toBe(after)
})
