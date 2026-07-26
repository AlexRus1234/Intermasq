// Templates modal (A7 smoke): open via the HostForm ⚙️ button, create a
// template, confirm it appears in the list, then delete it.
//
// A7 is classified cosmetic in tests/bugreport/bugs.md (not a bug), so this is a UI smoke,
// not a regression. Selectors: the ⚙️ entry button lives in the HostForm
// card; hostname_pattern has a hardcoded placeholder "device-{NNN}" (stable
// anchor), the rest of the create-form inputs are addressed positionally
// within .modal-content (ip_range stays a plain <input> because the e2e
// conf-dir has no dhcp-range directive → store.dhcpRanges is empty).

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
  // for canSubmit). hostname_pattern has a hardcoded placeholder.
  await modal.locator('input.form-control').nth(0).fill(NAME)
  await modal.locator('input.form-control').nth(1).fill('10.99.99.0/24')
  await modal.locator('input[placeholder="device-{NNN}"]').fill('e2e-{NNN}')
  await modal.locator('input.form-control').nth(3).fill(`${CONF_DIR}/e2e-tpl.conf`)
  await modal.locator('.btn-success').click()

  // Template shows up in the list.
  const item = modal.locator('.list-group-item', { hasText: NAME })
  await expect(item).toBeVisible({ timeout: 10000 })

  // Delete it (remove() uses confirm()).
  page.on('dialog', (d) => d.accept())
  await item.locator('button.btn-outline-danger').click()
  await expect(item).toBeHidden({ timeout: 10000 })
})
