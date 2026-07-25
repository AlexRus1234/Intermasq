// Shared API helpers for E2E specs: login + idempotent host seeding.
//
// Centralises the boilerplate that hosts-sort / host-crud / bulk-ops /
// search-filter all need (login once, seed a few hosts ignoring 409 on
// repeat runs). UI-driven specs (host-add-ui, host-tags, config-files)
// don't seed via API — they exercise the UI's own create flow.

import { request, expect } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:18083'
const ADMIN_USER = process.env.ADMIN_USER || 'admin'
const ADMIN_PASS = process.env.ADMIN_PASS || 'pass1234'

export const CONF_DIR = process.env.CONF_DIR!

export interface SeedHost {
  mac: string
  ip?: string
  hostname?: string
  file: string
  tags?: string[]
}

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

// seedHosts creates the given hosts via POST /api/hosts. Idempotent: a 409
// (mac_duplicate / ip_duplicate on a repeat run) is treated as success.
// Any other status throws — seeding must not fail silently.
export async function seedHosts(token: string, hosts: SeedHost[]): Promise<void> {
  const ctx = await request.newContext({ baseURL: BASE_URL })
  for (const h of hosts) {
    const res = await ctx.post('/api/hosts', {
      data: {
        mac: h.mac,
        ip: h.ip || '',
        hostname: h.hostname || '',
        file: h.file,
        tags: h.tags || [],
      },
      headers: { Authorization: `Bearer ${token}` },
    })
    if (res.status() !== 200 && res.status() !== 409) {
      throw new Error(`seed ${h.mac} failed: ${res.status()} ${await res.text()}`)
    }
  }
  await ctx.dispose()
}

// deleteHostApi removes a host via the API (used for afterAll cleanup if
// a spec wants to leave the shared conf-dir tidy; not required for CI,
// which starts fresh, but useful for local re-runs).
export async function deleteHostApi(
  token: string,
  mac: string,
  file: string,
): Promise<void> {
  const ctx = await request.newContext({ baseURL: BASE_URL })
  await ctx.delete(`/hosts/${mac}?file=${encodeURIComponent(file)}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  await ctx.dispose()
}
