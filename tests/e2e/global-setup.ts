// Global setup: wait for the intermasq binary, guarantee the admin account
// exists, obtain a JWT and write it into a storageState file that every spec
// (except auth.spec) reuses.
//
// We never launch a browser here — storageState is written directly as JSON.
// The localStorage origin MUST match baseURL exactly (incl. port) or the
// token will not apply to the page context.

import { request, expect } from '@playwright/test'
import { mkdirSync, writeFileSync } from 'node:fs'

const BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:18083'
const ADMIN_USER = process.env.ADMIN_USER || 'admin'
const ADMIN_PASS = process.env.ADMIN_PASS || 'pass1234'

// Poll /api/status until the server answers or we time out. The CI step
// already waits once before invoking playwright, but globalSetup is the
// authoritative gate: if the server isn't up, there's nothing to test.
async function waitForServer(url: string, timeoutMs = 30000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${url}/api/status`)
      if (res.ok) return
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error(`Server at ${url} did not become ready within ${timeoutMs}ms`)
}

export default async function globalSetup(): Promise<void> {
  await waitForServer(BASE_URL)

  const ctx = await request.newContext({ baseURL: BASE_URL })

  const statusRes = await ctx.get('/api/status')
  const status = await statusRes.json()

  // setup_required === true on a fresh user DB. POST /api/setup creates the
  // admin and returns a token; on a repeat run it 403s, so we branch on
  // setup_required rather than retrying setup.
  let token: string
  if (status.setup_required) {
    const r = await ctx.post('/api/setup', {
      data: { username: ADMIN_USER, password: ADMIN_PASS },
    })
    token = (await r.json()).token
  } else {
    const r = await ctx.post('/api/login', {
      data: { username: ADMIN_USER, password: ADMIN_PASS },
    })
    token = (await r.json()).token
  }

  expect(token).toBeTruthy()

  const origin = new URL(BASE_URL).origin
  mkdirSync('./.auth', { recursive: true })
  writeFileSync(
    './.auth/storageState.json',
    JSON.stringify(
      {
        cookies: [],
        origins: [
          {
            origin,
            localStorage: [{ name: 'token', value: token }],
          },
        ],
      },
      null,
      2,
    ),
  )

  await ctx.dispose()
}
