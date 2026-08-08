// Config template fill: creating a file from the `basic-dhcp` template via
// the config tab produces a new file tab whose raw content is the template
// skeleton (domain-needed / domain=lan / commented dhcp-range).
//
// Flow (DnsmasqConfig.vue): "+ new file" link (.nav-link.text-success) opens
// the .card.border-success form; enter a name, pick a template in the
// form's <select>, click ＋. createConfigFile (api/config.js) POSTs
// /api/config/file; the backend writes the template content verbatim (it
// is a skeleton — no dnsmasq --test at create time, see internal/dnsmasq/config_templates.go).
// On success the new file's tab appears and is auto-selected.
//
// We assert the tab shows up AND the raw content (GET /api/files/<name>)
// contains an active directive from the template ("domain-needed"). This
// spec does NOT save, so it works regardless of A13.

import { test, expect, request } from '@playwright/test'
import { apiLogin, BASE_URL, CONF_DIR } from '../lib/api'

const FILE_NAME = 'e2e-tpl-fill.conf'

test('config: file created from basic-dhcp template is filled with directives', async ({ page }) => {
  // One login + reusable context for the raw-content check.
  const token = await apiLogin()
  const ctx = await request.newContext({ baseURL: BASE_URL })

  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Open the config tab (⚙️ is the only .btn-group button carrying it).
  await page.locator('.btn-group button', { hasText: '⚙️' }).click()

  // "+ new file" link.
  await page.locator('.nav-link.text-success').click()

  const form = page.locator('.card.border-success')
  await expect(form).toBeVisible({ timeout: 5000 })
  await form.locator('input[placeholder="filename.conf"]').fill(FILE_NAME)
  // The template <select> is populated asynchronously by loadConfigTemplates
  // (fired, not awaited, in openNewFileForm), so wait for the option to land
  // before selecting it.
  await form.locator('option[value="basic-dhcp"]').waitFor({ state: 'attached', timeout: 10000 })
  await form.locator('select.form-select').selectOption('basic-dhcp')
  await form.locator('button.btn-success').click()

  // The new file's tab appears.
  await expect(page.locator('.nav-link', { hasText: FILE_NAME })).toBeVisible({ timeout: 10000 })

  // Raw content (verbatim template skeleton) contains an active directive.
  const res = await ctx.get(`/api/files/${FILE_NAME}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(res.ok(), `GET /api/files/${FILE_NAME} failed: ${res.status()}`).toBeTruthy()
  const body = await res.text()
  expect(body).toContain('domain-needed')
  await ctx.dispose()
})
