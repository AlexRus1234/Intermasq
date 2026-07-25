// Users tab: create a user via the UI, confirm it appears, delete it; and
// confirm deleting the current admin (self) is rejected by the backend
// (cannot_delete_self → the row stays).
//
// Selectors: the createUser card is the only .card.shadow-sm that contains
// a .btn-success; inside it username is the lone input[type="text"] and
// password is its input[type="password"]. The users list table renders one
// row per user with a 🗑 delete button (confirm-guarded).

import { test, expect } from '@playwright/test'

const NEW_USER = 'e2euser'
const NEW_PASS = 'pass1234'

async function openUsersTab(page: import('@playwright/test').Page) {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })
  await page.locator('.btn-group button', { hasText: '👥' }).click()
}

test('users: create then delete a user via UI', async ({ page }) => {
  await openUsersTab(page)

  // The createUser card is identified by its .btn-success Create button.
  const createCard = page.locator('.card.shadow-sm').filter({ has: page.locator('.btn-success') })
  await createCard.locator('input[type="text"]').fill(NEW_USER)
  await createCard.locator('input[type="password"]').fill(NEW_PASS)
  await createCard.locator('.btn-success').click()

  const row = page.locator('tbody tr', { hasText: NEW_USER })
  await expect(row).toBeVisible({ timeout: 10000 })

  // Delete the user we just created (deleteUser uses confirm()).
  page.on('dialog', (d) => d.accept())
  await row.locator('button.btn-outline-danger').click()
  await expect(row).toBeHidden({ timeout: 10000 })
})

test('users: deleting self is rejected', async ({ page }) => {
  await openUsersTab(page)

  const adminRow = page.locator('tbody tr', { hasText: 'admin' })
  await expect(adminRow).toBeVisible({ timeout: 10000 })

  // deleteUser() uses confirm(); on rejection it ALSO pops alert(msg)
  // (api/system.js). We must accept BOTH dialogs and wait for the error
  // alert — otherwise the alert fires after the test resolves and Playwright
  // throws "dialog.accept: page has been closed" during teardown.
  let dialogs = 0
  page.on('dialog', (d) => { dialogs++; d.accept() })
  await adminRow.locator('button.btn-outline-danger').click()
  await expect.poll(() => dialogs, { timeout: 10000 }).toBeGreaterThanOrEqual(2)

  // Rejected (cannot_delete_self): the admin row stays in the list.
  await expect(adminRow).toBeVisible({ timeout: 10000 })
})
