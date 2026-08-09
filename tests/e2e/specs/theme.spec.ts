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

// Theme toggle: clicking 🌓 flips data-bs-theme and persists to localStorage.
//
// Fresh storageState seeds only `token`, so on load localStorage.theme is
// null → App.vue onMounted applies no attribute (light). One toggle click
// must therefore land on `dark` and write localStorage.theme="dark".

import { test, expect } from '@playwright/test'

test('theme toggle switches data-bs-theme and persists', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await page.locator('.dropdown-toggle').click()
  await page.locator('.dropdown-item', { hasText: '🌓' }).click()

  // App.toggleTheme set the attribute to the opposite of the current value;
  // from a null/light start that is "dark".
  await expect(page.locator('html')).toHaveAttribute('data-bs-theme', 'dark')

  const stored = await page.evaluate(() => localStorage.getItem('theme'))
  expect(stored).toBe('dark')
})
