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

// Search/filter: typing into the dashboard search box filters the hosts
// table reactively (HostTable.sortedHosts filters by store.searchQuery).
//
// We seed 3 hosts with a unique prefix, confirm all 3 render, type the
// prefix (still 3), then a non-matching query (drops to 0). Counting only
// our prefix isolates the assertion from hosts left by other specs that
// share the conf-dir.
//
// Selector note: the search input (App.vue) has no hardcoded placeholder
// (it's i18n), so we target it structurally — it's the only input that is
// a direct child of a .col-md element (HostForm uses .col-md-3, which is a
// different class token and won't match .col-md).

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const PREFIX = 'aa:55:66:77:88'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: `${PREFIX}:01`, ip: '10.99.80.11', hostname: 'search-one', file: `${CONF_DIR}/e2e-search.conf` },
    { mac: `${PREFIX}:02`, ip: '10.99.80.12', hostname: 'search-two', file: `${CONF_DIR}/e2e-search.conf` },
    { mac: `${PREFIX}:03`, ip: '10.99.80.13', hostname: 'search-three', file: `${CONF_DIR}/e2e-search.conf` },
  ])
})

test('search filters rows by prefix', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const rows = page.locator('tbody tr td.font-monospace', { hasText: PREFIX })
  await expect(rows).toHaveCount(3, { timeout: 15000 })

  const search = page.locator('.col-md > input.form-control')

  // matching prefix → all 3 stay
  await search.fill(PREFIX)
  await expect(rows).toHaveCount(3)

  // non-matching → 0 of ours
  await search.fill('zz-no-such-host')
  await expect(rows).toHaveCount(0, { timeout: 10000 })
})
