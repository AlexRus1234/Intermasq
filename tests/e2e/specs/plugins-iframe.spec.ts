// Plugin iframe: the CI-built mock plugin "hello" (tests/fixtures/plugins/hello,
// installed to /etc/intermasq/plugins/hello before the e2e instance starts)
// appears in the user menu and opens in a full-screen overlay iframe.
//
// Flow (App.vue): the user menu lists store.plugins as 🧩 <name> dropdown
// items; clicking one sets store.tab='plugin-<id>', which renders the
// .plugin-overlay with <iframe src="/plugins/<id>/">. The reverse proxy
// (main.go::loadPlugins) forwards /plugins/hello/* to the plugin's unix
// socket. The mock serves JSON `{"plugin":"hello",...}` at "/", so the
// iframe body ends up containing "hello".
//
// The smoke suite already covers the proxy at the API level (82-plugins.sh);
// this spec covers the UI integration only.

import { test, expect } from '@playwright/test'

test('plugins: hello plugin opens in an iframe overlay', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Open the user menu and pick the 🧩 Hello Plugin entry.
  await page.locator('.dropdown-toggle').click()
  await page.locator('.dropdown-item', { hasText: '🧩' }).click()

  // Overlay + iframe render.
  const overlay = page.locator('.plugin-overlay')
  await expect(overlay).toBeVisible({ timeout: 5000 })
  const iframe = overlay.locator('iframe')
  await expect(iframe).toBeVisible()

  // The plugin returns JSON ({"plugin":"hello",...}); whatever the browser
  // wraps it in, "hello" ends up in the iframe body text.
  const frame = page.frameLocator('.plugin-overlay iframe')
  await expect(frame.locator('body')).toContainText('hello', { timeout: 10000 })
})
