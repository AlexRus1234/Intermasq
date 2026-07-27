// Skipped: the first-run setup screen only appears against a FRESH user DB,
// but the e2e instance (:18083) is already provisioned by global-setup, so
// /api/status reports setup_required=false and AuthScreen lands on "login",
// never "setup". Exercising setup needs a SECOND intermasq instance on
// :18084 with its own `-db /tmp/e2e-setup-users.json` (Gap_2_finish.md §6.7)
// — a CI-infrastructure change inside the opt-in L4 step that hasn't been
// added. Until it is, this spec is skipped. The login flow is already
// covered by auth.spec.

import { test } from '@playwright/test'

test.skip('setup-screen: first-run admin setup (needs isolated 2nd instance :18084)', () => {
  // To enable: in .forgejo/workflows/build.yml, inside the opt-in L4 step,
  // start a second intermasq-ci on :18084 against a fresh
  // -db /tmp/e2e-setup-users.json, then point this spec at
  // process.env.E2E_SETUP_BASE_URL and drive the setup form
  // (AuthScreen.vue: two input.form-control + .btn-primary).
})
