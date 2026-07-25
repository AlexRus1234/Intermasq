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
