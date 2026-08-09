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

// Templates modal (A7 smoke): open via the HostForm ⚙️ button, create a
// template, confirm it appears in the list, then delete it.
//
// A7 is classified cosmetic in tests/bugreport/bugs.md (not a bug), so this is a UI smoke,
// not a regression. P3.7: form inputs are addressed by data-testid (set in
// TemplatesModal.vue) instead of positional .nth() — the create form has 4
// inputs whose DOM order depends on store.dhcpRanges (ip_range renders as a
// <select> when dhcpRanges is non-empty), so positional indexes would silently
// drift and fill the wrong field. data-testid is order- and locale-independent
// (name/target_file placeholders are i18n-translated, so placeholder matchers
// would break under the RU locale).

import { test, expect } from '@playwright/test'
import { CONF_DIR } from '../lib/api'

const NAME = 'E2ETpl'

test('templates modal: create then delete a template', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Open TemplatesModal from the HostForm (the ⚙️ button inside the form card).
  await page.locator('.card.p-3.shadow-sm button', { hasText: '⚙️' }).click()
  const modal = page.locator('.modal-content')
  await expect(modal).toBeVisible({ timeout: 5000 })

  // Create form: name, ip_range, hostname_pattern, target_file (all required
  // for canSubmit). Anchored on data-testid, not DOM position.
  await modal.locator('[data-testid="tpl-name"]').fill(NAME)
  await modal.locator('[data-testid="tpl-ip-range"]').fill('10.99.99.0/24')
  await modal.locator('[data-testid="tpl-hostname-pattern"]').fill('e2e-{NNN}')
  await modal.locator('[data-testid="tpl-target-file"]').fill(`${CONF_DIR}/e2e-tpl.conf`)
  await modal.locator('.btn-success').click()

  // Template shows up in the list.
  const item = modal.locator('.list-group-item', { hasText: NAME })
  await expect(item).toBeVisible({ timeout: 10000 })

  // Delete it (remove() uses confirm()).
  page.on('dialog', (d) => d.accept())
  await item.locator('button.btn-outline-danger').click()
  await expect(item).toBeHidden({ timeout: 10000 })
})
