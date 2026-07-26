// A1 regression: sorting the hosts table must not change the number of
// rendered rows for our seeded prefix.
//
// Root cause of A1 (tests/bugreport/bugs.md): HostTable.vue uses `:key="h.mac"` which
// is not guaranteed unique, so on re-sort Vue can reuse DOM and rows appear
// duplicated. For this guard the mechanism is irrelevant — the invariant
// is "row count for our prefix is stable across sort clicks".
//
// We seed 5 hosts with a unique prefix and count ONLY matching rows so
// leftover hosts from other specs (which share the conf-dir) can't skew
// the count.

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const PREFIX = 'aa:11:11:11:11'

test.beforeAll(async () => {
  const token = await apiLogin()
  const file = `${CONF_DIR}/e2e-sort.conf`
  await seedHosts(
    token,
    [1, 2, 3, 4, 5].map((i) => ({
      mac: `${PREFIX}:${String(i).padStart(2, '0')}`,
      ip: `10.99.${i}.2`,
      hostname: `sort${i}`,
      file,
    })),
  )
})

test('A1 regression: sorting does not change row count', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const rows = page.locator('tbody tr td.font-monospace', { hasText: PREFIX })
  await expect(rows).toHaveCount(5, { timeout: 15000 })

  for (let i = 0; i < 3; i++) {
    await page.locator('th', { hasText: 'IP' }).click()
    await expect(rows).toHaveCount(5)
  }
})
