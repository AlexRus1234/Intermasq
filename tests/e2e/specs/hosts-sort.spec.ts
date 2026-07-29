// A1 regression: sorting the hosts table must not change the number of
// rendered rows for our seeded prefix AND must reflect the correct order
// after each sort click.
//
// Root cause of A1 (tests/bugreport/bugs.md): HostTable.vue used `:key="h.mac"`
// which is not guaranteed unique, so on re-sort Vue could reuse DOM and rows
// appeared duplicated. The earlier guard only checked `toHaveCount(5)`, which
// a mutation-pass (логи/gap2-finish.md Блок C) showed still passes with broken
// sort code — so it was a guard, not a regression. This spec now also asserts
// the visible ORDER after each click, turning it into a real regression.
//
// Sort contract (HostTable.vue:82-95): sortKey starts at 'ip', sortAsc=true.
// Clicking the SAME key toggles sortAsc; clicking a NEW key sets sortAsc=true.
//
// We seed 5 hosts with a unique prefix and match ONLY those rows (via the MAC
// cell `.font-monospace`) so leftover hosts from other specs (which share the
// conf-dir) can't skew the count or order.

import { test, expect, type Page } from '@playwright/test'
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

// visibleOrder returns the MAC postfix (last 2 chars) of our seeded rows in
// DOM order — i.e. the order they are currently rendered in.
async function visibleOrder(page: Page): Promise<string[]> {
  const cells = await page.locator('tbody tr td.font-monospace', { hasText: PREFIX }).all()
  const texts = await Promise.all(cells.map((c) => c.textContent()))
  return texts.map((t) => (t ?? '').slice(-2))
}

test('A1 regression: sorting preserves row count and order', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const rows = page.locator('tbody tr td.font-monospace', { hasText: PREFIX })
  await expect(rows).toHaveCount(5, { timeout: 15000 })

  // Initial mount: sortKey='ip', sortAsc=true → ascending by IP (10.99.1..5.2).
  expect(await visibleOrder(page)).toEqual(['01', '02', '03', '04', '05'])

  // 1) Click IP → same key toggles sortAsc to false → descending.
  await page.locator('th', { hasText: 'IP' }).click()
  expect(await visibleOrder(page)).toEqual(['05', '04', '03', '02', '01'])
  await expect(rows).toHaveCount(5)

  // 2) Click IP again → ascending.
  await page.locator('th', { hasText: 'IP' }).click()
  expect(await visibleOrder(page)).toEqual(['01', '02', '03', '04', '05'])
  await expect(rows).toHaveCount(5)

  // 3) Click Hostname → new key → sortAsc=true → ascending by hostname sort1..5.
  await page.locator('th', { hasText: 'Hostname' }).click()
  expect(await visibleOrder(page)).toEqual(['01', '02', '03', '04', '05'])
  await expect(rows).toHaveCount(5)

  // 4) Click Hostname again → descending.
  await page.locator('th', { hasText: 'Hostname' }).click()
  expect(await visibleOrder(page)).toEqual(['05', '04', '03', '02', '01'])
  await expect(rows).toHaveCount(5)
})
