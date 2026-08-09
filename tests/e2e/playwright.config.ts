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
