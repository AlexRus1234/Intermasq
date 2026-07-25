// Discovery tab: an ARP device that is neither in static hosts nor in
// leases must show up under "Unknown devices", and clicking its ➕ must
// hand it off to the static form (store.transferData + tab='static').
//
// The CI arp-fixture (tests/fixtures/arp-sample.txt) has 11:22:33:44:55:01;
// none of the e2e specs seed it as a static host, so it's "unknown". The
// leases fixture is empty, so the "new leases" section is irrelevant.

import { test, expect } from '@playwright/test'

const ARP_MAC = '11:22:33:44:55:01' // from tests/fixtures/arp-sample.txt

test('discovery: unknown ARP device listed; ➕ switches to static', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await page.locator('.btn-group button', { hasText: '🔍' }).click()

  // newDevices lives in the .border-warning card.
  const newDevCard = page.locator('.card.border-warning')
  const deviceRow = newDevCard.locator('tbody tr', { hasText: ARP_MAC })
  await expect(deviceRow).toBeVisible({ timeout: 15000 })

  // ➕ → addToStatic → store.transferData set, tab flips to 'static', and
  // HostForm is prefilled with the MAC.
  await deviceRow.locator('button.btn-outline-primary').click()
  await expect(page.locator('input[placeholder="MAC (aa:bb...)"]')).toHaveValue(ARP_MAC)
})
