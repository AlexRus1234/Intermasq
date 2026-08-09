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

// Config directive save validates via dnsmasq --test --conf-file=<path>
// (the A13 fix). We exercise the visual editor end-to-end:
//   1. Create e2e-directive.conf from the `basic-dhcp` template — that gives
//      a "dns" group card (domain-needed / bogus-priv / expand-hosts /
//      domain=lan) so the per-group "+ add directive" button is present
//      (an empty file would have NO group cards and thus no add button).
//   2. Add a custom `port` directive (schemaFor('port') → 'other' group) with
//      a VALID value 5353, save → PUT /api/config → writeConfigWithTest →
//      dnsmasq --test passes → file written. Assert raw content has port=5353.
//   3. Edit the same directive to an INVALID value `abc`, save → dnsmasq
//      --test rejects → 400 dnsmasq_test_failed → saveConfig shows alert(),
//      backend rolls the file back. Assert raw content still has port=5353
//      (NOT port=abc).
//
// save() always fires a confirm() dialog; a FAILED save additionally fires
// an alert() with the translated error + dnsmasq output. We auto-accept all
// dialogs. File-content assertions go through the API (locale-independent).
// We log in once and reuse the request context (apiLogin is rate-limited).

import { test, expect, request } from '@playwright/test'
import { apiLogin, BASE_URL } from '../lib/api'

const FILE_NAME = 'e2e-directive.conf'

test('config-directive: save validates via dnsmasq --test (A13); invalid value is rejected and rolled back', async ({ page }) => {
  // save() confirm() + failure alert() — accept both, in order.
  page.on('dialog', (d) => d.accept())

  // One login + reusable context for the API file-content checks.
  const token = await apiLogin()
  const ctx = await request.newContext({ baseURL: BASE_URL })
  const rawContent = async (): Promise<string> => {
    const res = await ctx.get(`/api/files/${FILE_NAME}`, { headers: { Authorization: `Bearer ${token}` } })
    if (!res.ok()) return ''
    return res.text()
  }

  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })
  await page.locator('.btn-group button', { hasText: '⚙️' }).click()

  // --- create e2e-directive.conf from basic-dhcp ---
  await page.locator('.nav-link.text-success').click()
  const createForm = page.locator('.card.border-success')
  await expect(createForm).toBeVisible({ timeout: 5000 })
  await createForm.locator('input[placeholder="filename.conf"]').fill(FILE_NAME)
  // The template <select> is populated asynchronously by loadConfigTemplates
  // (fired, not awaited, in openNewFileForm), so wait for the option to land.
  await createForm.locator('option[value="basic-dhcp"]').waitFor({ state: 'attached', timeout: 10000 })
  await createForm.locator('select.form-select').selectOption('basic-dhcp')
  await createForm.locator('button.btn-success').click()
  await expect(page.locator('.nav-link', { hasText: FILE_NAME })).toBeVisible({ timeout: 10000 })

  // --- add a custom `port` directive via the dns group's "+ add directive" ---
  // basic-dhcp parses into dns + dhcp groups (the commented #dhcp-range /
  // #dhcp-option lines become inactive dhcp directives, not skipped), so
  // there are several .btn-outline-primary add buttons across group cards.
  // Scope to the FIRST group card (GROUP_ORDER = dns, dhcp, …), which is the
  // dns group and carries exactly one add-directive button.
  await page.locator('.card.mb-3.shadow-sm').first().locator('button.btn-outline-primary').click()
  const addPanel = page.locator('.card.border-primary')
  await expect(addPanel).toBeVisible({ timeout: 5000 })
  await addPanel.locator('input.form-control-sm').fill('port')
  await addPanel.locator('button.btn-primary').click()

  // The new `port` directive lands in the 'other' group card. Fill its value
  // input (the row containing <code>port</code>).
  const portCard = page.locator('.card.mb-3.shadow-sm', { has: page.locator('code', { hasText: 'port' }) })
  await expect(portCard).toBeVisible({ timeout: 5000 })
  await portCard.locator('input.form-control-sm').fill('5353')

  // --- save (valid) → PUT /api/config 200, dnsmasq --test passes ---
  await Promise.all([
    page.waitForResponse(
      (r) => r.request().method() === 'PUT' && r.url().endsWith('/api/config'),
      { timeout: 15000 },
    ),
    page.locator('button.btn-primary', { hasText: '💾' }).click(),
  ])
  await expect.poll(() => rawContent(), { timeout: 15000 }).toContain('port=5353')

  // --- edit to invalid value `abc`, save → PUT /api/config 400 + rollback ---
  await portCard.locator('input.form-control-sm').fill('abc')
  await Promise.all([
    page.waitForResponse(
      (r) => r.request().method() === 'PUT' && r.url().endsWith('/api/config') && r.status() === 400,
      { timeout: 15000 },
    ),
    page.locator('button.btn-primary', { hasText: '💾' }).click(),
  ])
  // 400 → backend rolled the file back to the port=5353 state before responding.
  const after = await rawContent()
  expect(after).not.toContain('port=abc')
  expect(after).toContain('port=5353')

  await ctx.dispose()
})
