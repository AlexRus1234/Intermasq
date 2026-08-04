# P3 — polish тестовой инфраструктуры до v1.0 (бывший P2) ✓ ВЫПОЛНЕНО

**Статус:** ✓ ВЫПОЛНЕНО (2026-08-04, коммиты `18825b1` + `b5df99e`, push в `main`;
CI Fedora 44 green).
**Подробный лог:** [`predrel-test-remediation-p3-exec.md`](./predrel-test-remediation-p3-exec.md).

---

## Сводка по задачам

| Задача | Статус | Кратко |
|--------|:------:|--------|
| **P3.1** | ✓ | doc-комментарий к `TestIsSafePath` — прямой тест + prefix-collision case (`_evil`) **уже существовали**; comments «substring fires first» (Variant A) тоже. Реальный пробел — только doc. Мутация (убрать `+string(os.PathSeparator)`) поймана + откатана |
| **P3.2** | ✓ | `fakeDnsmasqStrict` (парсит `--conf-file=`, rejects `# INVALID`) + `TestWriteConfigWithTest_StrictFakeRejectsInvalid`; wiring зеркально `fakeDnsmasq` (промт's `withDnsmasqBin` не существует). Wiring + content-валидация совместно |
| **P3.3** | ✓ | `80-metrics.sh` A8-инверсия починена: honest regression (body >2 AND `auth_required`); тег A8 + `\|\| true` сняты (A8 FIXED, нет в known-bugs) |
| **P3.4** | ✓ | `11-auth-ratelimit.sh` RL_BLOCKED-aware: else-ветка ассертит 401 (медленный CI / протухшее окно); `\|\| true` снят |
| **P3.5** | ✓ | `grep -c \|\| echo 0` → `\|\| true` в `20-hosts-happy.sh` / `31-aliases-bugs.sh` (давало `"0\n0"`); изолированным bash-тестом верифицировано |
| **P3.6** | ✓ | `audit-tab.spec.ts`: clean-slate `deleteHostApi` + per-run-unique hostname (action badge i18n-переведён → raw-матч «add» нерабочий); ловит writeAudit no-op |
| **P3.7** | ✓ | `templates-modal.spec.ts` + `TemplatesModal.vue`: 4× `data-testid` вместо позиционных `.nth()` (order/locale-independent; placeholders name/target_file i18n). Правка products-кода additive |
| **P3.8** | ✓ | 5 новых smoke-suites: `28` apply-template, `44` leases-to-static, `84` restart-self, `85` reload, `86` events-sse. `GET /api/aliases` уже покрыт (P2.1). 84/85/86 — **до** `90-logout` (после него JWT невалиден, нумерация промта 91/92/93 была бы сломана) |
| **P3.9** | ✓ | `ROADMAP.md` vanity-заметка в Gap 4 (system_callers_test.go = statement-%, rely on L5); VANITY-комментарий в `system_callers_test.go:19` уже был |
| **P3.10** | ✓ | `go vet`/`test`/`-race` (107.997с, 0 races); `bash -n` 9 suite'ов; `vite build` 121; `playwright --list` 34; мутация P3.1 поймана |

## Расхождения с промтом (исправлено по ходу)

Полный разбор — в execution-логе §«Расхождения с промт-планом». Кратко: в 6 из 9
задач найдены неточности —

- **P3.1:** прямой тест `isSafePath` **уже существовал** с prefix-collision case;
  comments (Variant A) тоже. Реальный пробел — только doc-комментарий.
- **P3.2:** `withDnsmasqBin` не существует — внутренний wiring зеркально `fakeDnsmasq`.
- **P3.6:** action badge i18n-переведён (`audit.action_add`=«Добавление»/«Add») →
  raw-матч «add» нерабочий; нужен non-locale дискриминатор (hostname) + clean-slate.
- **P3.7:** placeholders name/target_file i18n-переведены → `data-testid`
  (preferred-вариант промта); правка products-кода additive.
- **P3.8:** `GET /api/aliases` уже покрыт (P2.1) — пропущен; нумерация suite'ов
  84/85/86 (не 91/92/93), т.к. после `90-logout` JWT невалиден; `reload`
  ослаблен до 200\|400.
- **P3.9:** VANITY-комментарий в `system_callers_test.go:19` уже на месте —
  только заметка в ROADMAP.

P3.3, P3.4, P3.5 — допущения подтверждены дословно.

## Осталось (вне P3)

- **Продуктовый security-аудит** (вне тестовых промтов): JWT alg-confusion в
  `auth.go:214`, plugin trust boundary в `main.go:131-193`, X-Forwarded-For в
  `rateLimitMiddleware`, `hash, _ :=` в `handlers.go:47`.
- **A15** — KNOWN-CONDITIONAL на dnsmasq 2.80 по решению оператора.
- **v1.0 release** — CHANGELOG.md / README (последние unchecked чекбоксы в
  `tests/ROADMAP.md:199-200`).
