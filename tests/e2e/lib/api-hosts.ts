// Host seeding helpers for E2E specs. See api-auth.ts for env/login.

import { request } from '@playwright/test'
import { BASE_URL } from './api-auth'

export interface SeedHost {
  mac: string
  ip?: string
  hostname?: string
  file: string
  tags?: string[]
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
