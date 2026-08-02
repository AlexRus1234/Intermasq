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
  // De-correlated seed (predrel-test-remediation-P1 §P1.5): IP and hostname
  // grow in OPPOSITE directions, so ascending-by-IP != ascending-by-hostname.
  // A mutation that breaks sortKey switching (e.g. dropping `sortKey.value =
  // key` in HostTable.vue:94) would otherwise leave ORDER assertions identical
  // for both sort keys and slip through. MAC postfix 01..05 is preserved so
  // visibleOrder() and the `hasText: PREFIX` filter keep working unchanged.
  await seedHosts(token, [
    { mac: `${PREFIX}:01`, ip: '10.99.5.2', hostname: 'sortA', file },
    { mac: `${PREFIX}:02`, ip: '10.99.4.2', hostname: 'sortB', file },
    { mac: `${PREFIX}:03`, ip: '10.99.3.2', hostname: 'sortC', file },
    { mac: `${PREFIX}:04`, ip: '10.99.2.2', hostname: 'sortD', file },
    { mac: `${PREFIX}:05`, ip: '10.99.1.2', hostname: 'sortE', file },
  ])
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

  // Initial mount: sortKey='ip', sortAsc=true → ascending by IP. With our
  // de-correlated seed, smallest IP (10.99.1.2) belongs to suffix 05, so
  // ascending-by-IP yields suffix order ['05'..'01'] (NOT the MAC/seed order).
  expect(await visibleOrder(page)).toEqual(['05', '04', '03', '02', '01'])

  // 1) Click IP → same key toggles sortAsc to false → descending. Largest IP
  // (10.99.5.2) is suffix 01, so descending-by-IP yields ['01'..'05'].
  await page.locator('th', { hasText: 'IP' }).click()
  expect(await visibleOrder(page)).toEqual(['01', '02', '03', '04', '05'])
  await expect(rows).toHaveCount(5)

  // 2) Click IP again → ascending (same as initial mount).
  await page.locator('th', { hasText: 'IP' }).click()
  expect(await visibleOrder(page)).toEqual(['05', '04', '03', '02', '01'])
  await expect(rows).toHaveCount(5)

  // 3) Click Hostname → new key → sortAsc=true → ascending by hostname. sortA
  // is suffix 01 and sortE is suffix 05, so ascending-by-hostname yields
  // ['01'..'05'] — CRITICALLY DIFFERENT from ascending-by-IP (['05'..'01']).
  // This is the assertion that catches a `sortKey.value = key` regression in
  // HostTable.vue: if the key never switches, the actual order here would be
  // the IP-ascending ['05'..'01'] and this expect would fail.
  await page.locator('th', { hasText: 'Hostname' }).click()
  expect(await visibleOrder(page)).toEqual(['01', '02', '03', '04', '05'])
  await expect(rows).toHaveCount(5)

  // 4) Click Hostname again → descending. sortE (suffix 05) first, sortA
  // (suffix 01) last → ['05'..'01'].
  await page.locator('th', { hasText: 'Hostname' }).click()
  expect(await visibleOrder(page)).toEqual(['05', '04', '03', '02', '01'])
  await expect(rows).toHaveCount(5)
})
