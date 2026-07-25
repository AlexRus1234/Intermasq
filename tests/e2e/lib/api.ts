// Barrel for E2E API helpers. Domain implementations live in sibling
// api-*.ts files (api-auth, api-hosts, ...). Specs import from '../lib/api'
// — this file keeps that path stable as new domains are added.

export * from './api-auth'
export * from './api-hosts'
