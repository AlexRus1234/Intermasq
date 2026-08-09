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

// Reload via the toolbar "🔄 Apply": applyConfig() POSTs /api/reload and
// always pops an alert (success or error). We accept the alert and assert
// the response is 200 — that means `dnsmasq --test` passed and the init
// caller's restart succeeded (with -init-system=none the restart is a
// no-op, per checklist §12.3 / smoke). Response status is the clean
// success gate since the alert text is locale-dependent.

import { test, expect } from '@playwright/test'

test('reload via Apply returns 200', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // applyConfig() alerts on both success and error; accept unconditionally.
  page.on('dialog', (d) => d.accept())

  // P2.3: do NOT filter waitForResponse on status === 200 — if applyConfig
  // returns 400 (e.g. dnsmasq --test failed), the predicate never matches,
  // waitForResponse hangs for the full 30s test timeout and then fails with
  // an opaque message. Match any /api/reload response with an explicit
  // shorter timeout, then assert status below so a 400 surfaces the real
  // status code quickly and readably.
  const [resp] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().includes('/api/reload'),
      { timeout: 15000 },
    ),
    page.locator('.btn-warning', { hasText: '🔄' }).click(),
  ])
  expect(resp.status()).toBe(200)
})
