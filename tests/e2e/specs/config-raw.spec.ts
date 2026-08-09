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

// Raw text editor (PUT /api/files/:name → writeFileRaw) driven through the
// UI that the visual/raw toggle in DnsmasqConfig.vue exposes.
//
// Previously this spec was permanently skipped because no raw UI existed —
// the backend path was covered only by smoke (suite 40-config-files.sh) +
// L1/L2 Go tests. The Visual/Raw toggle added to DnsmasqConfig.vue lifts
// that blocker, so the skip is removed and this spec now exercises the
// same A13 guarantee end-to-end through the textarea:
//   1. valid content → PUT 200, dnsmasq --test passes → file written.
//   2. invalid content (port=abc) → PUT 400, file rolled back from .bak so
//      the last valid content survives.
//
// Mirrors config-directive.spec.ts structure (apiLogin + request context
// for locale-independent content assertions; auto-accept confirm()/alert()
// dialogs fired by save() and the failed-save alert). The file is seeded
// via the API; a 409 on local re-runs (file already exists) is fine — the
// raw PUT overwrites whatever was there.

import { test, expect, request } from '@playwright/test'
import { apiLogin, BASE_URL } from '../lib/api'

const FILE_NAME = 'e2e-raw.conf'

test('config-raw: textarea save validates via dnsmasq --test; invalid content is rolled back (A13)', async ({ page }) => {
  // save() confirm() + failed-save alert() — accept both, in order.
  page.on('dialog', (d) => d.accept())

  const token = await apiLogin()
  const ctx = await request.newContext({ baseURL: BASE_URL })

  // Seed the file (ignore 409 on local re-runs). CI starts with a fresh
  // conf-dir so the create always succeeds there.
  await ctx.post('/api/config/file', {
    data: { name: FILE_NAME, template: 'empty' },
    headers: { Authorization: `Bearer ${token}` },
  })

  const rawContent = async (): Promise<string> => {
    const res = await ctx.get(`/api/files/${FILE_NAME}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok()) return ''
    return (await res.json()).content
  }

  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Config tab: the btn-group button carrying ⚙️ (scoped to btn-group so
  // the dropdown-toggle ⚙️ menu button can't match).
  await page.locator('.btn-group button', { hasText: '⚙️' }).click()

  // Select our file so the per-file toolbar (mode toggle + textarea) renders.
  const tab = page.locator('.nav-link', { hasText: FILE_NAME })
  await expect(tab).toBeVisible({ timeout: 10000 })
  await tab.click()

  // Switch to Raw mode. switchMode('raw') fires loadRaw() → GET
  // /api/files/:name; wait for that response so the textarea is populated
  // before we fill it (otherwise the GET could overwrite our input).
  await Promise.all([
    page.waitForResponse(
      (r) => r.request().method() === 'GET' && r.url().includes(`/api/files/${FILE_NAME}`),
      { timeout: 15000 },
    ),
    page.locator('.btn-group-sm button', { hasText: '📝' }).click(),
  ])
  const textarea = page.locator('textarea')
  await expect(textarea).toBeVisible({ timeout: 5000 })

  // --- valid content → PUT 200, dnsmasq --test passes ---
  await textarea.fill('# e2e raw\ndomain-needed\nbogus-priv\n')
  await Promise.all([
    page.waitForResponse(
      (r) =>
        r.request().method() === 'PUT' &&
        r.url().includes(`/api/files/${FILE_NAME}`) &&
        r.status() === 200,
      { timeout: 15000 },
    ),
    page.locator('button.btn-primary', { hasText: '💾' }).click(),
  ])
  await expect.poll(async () => rawContent(), { timeout: 15000 }).toContain('domain-needed')

  // --- invalid content → PUT 400 + .bak rollback (A13) ---
  await textarea.fill('# invalid\nport=abc\n')
  await Promise.all([
    page.waitForResponse(
      (r) =>
        r.request().method() === 'PUT' &&
        r.url().includes(`/api/files/${FILE_NAME}`) &&
        r.status() === 400,
      { timeout: 15000 },
    ),
    page.locator('button.btn-primary', { hasText: '💾' }).click(),
  ])
  // 400 → backend rolled the file back to the last valid state before
  // responding, so port=abc never lands and domain-needed survives.
  const after = await rawContent()
  expect(after).not.toContain('port=abc')
  expect(after).toContain('domain-needed')

  await ctx.dispose()
})
