// Shared E2E API helpers — auth + env.
//
// Split out of the original api.ts so the module doesn't grow unbounded as
// more domains are added (hosts already lives in api-hosts.ts; aliases will
// get api-aliases.ts in batch 3 phase B). api.ts remains a barrel that
// re-exports everything, so spec imports (`../lib/api`) stay unchanged.

import { request, expect } from '@playwright/test'

export const BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:18083'
export const CONF_DIR = process.env.CONF_DIR!

const ADMIN_USER = process.env.ADMIN_USER || 'admin'
const ADMIN_PASS = process.env.ADMIN_PASS || 'pass1234'

// apiLogin authenticates against /api/login and returns a JWT. Each spec
// that needs to seed calls this in beforeAll. /api/login is rate-limited
// to 10/min, but a successful login resets the counter (auth.go), so a
// handful of beforeAll logins per run stay well under the cap.
export async function apiLogin(): Promise<string> {
  const ctx = await request.newContext({ baseURL: BASE_URL })
  const r = await ctx.post('/api/login', {
    data: { username: ADMIN_USER, password: ADMIN_PASS },
  })
  expect(r.ok(), `login failed: ${r.status()} ${await r.text()}`).toBeTruthy()
  const { token } = await r.json()
  await ctx.dispose()
  return token
}
