// Playwright config for Intermasq E2E (Gap 2).
//
// Design constraints (see логи/Gap_2.md §4.2 / §6):
//  - workers:1 + fullyParallel:false — the specs share one conf-dir and one
//    dnsmasq hosts file; parallelism would race. /api/login is also rate-
//    limited to 10/min, so logins must stay sequential.
//  - globalSetup seeds an admin JWT into storageState so every spec starts
//    already-authenticated (auth.spec opts out via an empty storageState).
//  - baseURL / conf-dir / credentials come from env set by the CI step.

import { defineConfig, devices } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:18083'

export default defineConfig({
  testDir: './specs',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  globalSetup: './global-setup.ts',
  use: {
    baseURL: BASE_URL,
    storageState: './.auth/storageState.json',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
})
