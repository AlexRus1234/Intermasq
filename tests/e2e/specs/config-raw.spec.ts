// Skipped: the raw-file PUT path (PUT /api/files/<name> → writeFileRaw) is
// already covered by smoke.sh suite 40-config-files.sh, including the A13
// behaviour (invalid syntax → 400 + rollback; valid syntax → 200). An E2E
// twin would duplicate those API checks without adding UI coverage — there
// is no raw-edit UI, only the visual editor (DnsmasqConfig.vue), which is
// covered by config-directive.spec.ts. Gap_2_finish.md §6.6 explicitly
// allows skipping ("по сути дублирует smoke — оцени целесообразность").

import { test } from '@playwright/test'

test.skip('config-raw: PUT /api/files raw valid/invalid (covered by smoke 40-config-files)', () => {
  // Unskip only if a UI affordance for raw editing is added, or if smoke
  // coverage of this path is removed.
})
