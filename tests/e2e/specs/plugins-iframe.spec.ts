// Plugin iframe: the CI-built mock plugin "hello" (tests/fixtures/plugins/hello,
// installed to /etc/intermasq/plugins/hello before the e2e instance starts)
// appears in the user menu and opens in a full-screen overlay iframe.
//
// Flow (App.vue): the user menu lists store.plugins as 🧩 <name> dropdown
// items; clicking one sets store.tab='plugin-<id>', which renders the
// .plugin-overlay with <iframe src="/plugins/<id>/">. The reverse proxy
// (internal/plugins.Load) forwards /plugins/hello/* to the plugin's unix
// socket. The mock serves JSON `{"plugin":"hello",...}` at "/", so the
// iframe body ends up containing "hello".
//
// The smoke suite already covers the proxy at the API level (82-plugins.sh);
// this spec covers the UI integration only.

import { test, expect } from '@playwright/test'

test('plugins: hello plugin opens in an iframe overlay', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // P2.4: open the user menu and count 🧩 entries. App.vue renders one
  // dropdown item per loaded plugin, so zero 🧩 items means no plugin is
  // installed (local run, or a CI matrix without the Gap 6 mock-plugin
  // install step). Skip cleanly instead of locator-timeout'ing on the
  // missing item. store.plugins loads on app mount; if the dropdown beat
  // the fetch, re-check after a short wait before deciding.
  await page.locator('.dropdown-toggle').click()
  const pluginItems = page.locator('.dropdown-item', { hasText: '🧩' })
  let count = await pluginItems.count()
  if (count === 0) {
    await page.waitForTimeout(1500)
    count = await pluginItems.count()
  }
  test.skip(count === 0, 'no plugins loaded — requires CI mock plugin (Gap 6)')

  // Open the plugin overlay.
  await pluginItems.first().click()

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
