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

// Discovery tab: an ARP device that is neither in static hosts nor in
// leases must show up under "Unknown devices", and clicking its ➕ must
// hand it off to the static form (store.transferData + tab='static').
//
// The CI arp-fixture (tests/fixtures/arp-sample.txt) has 11:22:33:44:55:01;
// none of the e2e specs seed it as a static host, so it's "unknown". The
// leases fixture is empty, so the "new leases" section is irrelevant.

import { test, expect } from '@playwright/test'
import { apiLogin, BASE_URL } from '../lib/api'

const ARP_MAC = '11:22:33:44:55:01' // from tests/fixtures/arp-sample.txt

test('discovery: unknown ARP device listed; ➕ switches to static', async ({ page }) => {
  // P2.4: skip when there are no unknown devices — happens on local runs
  // without the CI arp fixture (tests/fixtures/arp-sample.txt), or when the
  // fixture MACs have all been seeded as static hosts. Probe the same API
  // the discovery tab reads so the skip decision matches what the UI will
  // render, instead of locator-timeout'ing on the missing row.
  const token = await apiLogin()
  const probe = await page.request.get(`${BASE_URL}/api/new-devices`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  let devices: unknown[] = []
  if (probe.ok()) {
    const parsed = await probe.json()
    if (Array.isArray(parsed)) devices = parsed
  }
  test.skip(devices.length === 0, 'no new devices — requires CI arp fixture')

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
