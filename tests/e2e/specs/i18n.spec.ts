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
