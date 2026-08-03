# P2 — починки перед глубоким рефакторингом (бывший P1) ✓ ВЫПОЛНЕНО

**Статус:** ✓ ВЫПОЛНЕНО (2026-08-03, коммиты `8a489f3` + `04ad007` + `c52c3b0`
docs, push в `main`; + 2 follow-up `a8998e0` / `9f2a447` после CI).
**Подробный лог:** [`predrel-test-remediation-p2-exec.md`](./predrel-test-remediation-p2-exec.md).

---

## Сводка по задачам

| Задача | Статус | Кратко |
|--------|:------:|--------|
| **P2.1** | ✓ | `check_length` (jq `>=`) + `check_lines` (wc -l `>=`) хелперы в `tests/lib/http.sh`; обёрнуты уже-вычисляемые counts в 22/30/40/50 + новый `GET /api/aliases` в 30; CSV 25/34 через `check_lines`; known-empty (42 ranges, 83 leases) — комментарии; 83 next-ip — non-empty `.ip`. 70-audit/ARP не тронуты (уже ассерчены) |
| **P2.2** | ✓ | Self-seed 26/27: пересоздают `aa:bb:cc:dd:ee:10` / `ee:11`(csv2/10.0.0.21) в `$FILE`; `200\|409` приняты; больше не зависят от 23 |
| **P2.3** | ✓ | `reload-ui.spec.ts` `waitForResponse` без фильтра по status (вис 30с на 400); timeout 15с + явный `expect(resp.status()).toBe(200)` |
| **P2.4** | ✓ | Runtime skip-guards: `plugins-iframe` (подсчёт 🧩 dropdown-items), `discovery-tab` (probe `/api/new-devices` через `apiLogin`). Без правок CI env |
| **P2.5** | ✓ | `sysCaller` добавлен в snapshot/restore `withSandboxFlags` (`setup_test.go`); иллюстрация `TestWithSandboxFlags_RestoresSysCaller` (sentinel `SystemdSystemCaller` для pointer-identity) |
| **P2.6** | ✓ | `TestSetupServer_HistoryDirFail` обёрнут в `withSandboxFlags` → `loadTemplates` `os.Exit(1)` на хостовом `/etc/intermasq/templates.json` больше не убивает тест-бинарник |
| **P2.7** | ✓ | `TestMetricsHandler_AllMetricNames` — e2e `metricsHandler`, пинит все 9 имён метрик; dnsHealth + `NoneCaller` засеяны (без `setupServer` `checkDnsmasqStatus` nil-deref'ит) |
| **P2.8** | ✓ | Новый `audit_test.go`: `TestWriteAudit_Concurrent` (100 горутин, валидный JSON) + `TestWriteAudit_ReadRoundtrip` (write→`auditHandler` GET) |
| **P2.9** | ✓ | `FuzzParseLeasesContent` — тавтология (empty-string) заменена **структурным** инвариантом field-index mapping + count. Первая попытка (format-gate) крашилась на `"0 : 0"` — см. follow-up |
| **P2.10** | ✓ | `users-tab.spec.ts` self-delete: реальный `expect(resp.status()).toBe(400)` + `body.error` contains `cannot_delete_self` вместо dialog-count |
| **P2.11** | ✓ | `HistoryModal.vue` `<tr>` → `:data-version="v.version"`; spec выбирает старейший snapshot (lexicographic-min `data-version`) — order-independent |
| **P2.12** | ✓ | `go vet`/`build`/`test`/`-race` (125с, 0 races); `bash -n` чисто; `vite build` ок; `playwright --list` 34 specs; мутации P2.7/P2.8/P2.9 пойманы |

## Follow-up фиксы (после CI)

- **`a8998e0`** — P2.9 fuzz-gate (`ContainsAny ":-"`) крашился на CI-входе
  `"0 : 0"`. `parseLeasesContent` принимает мусор → любой format-ассерт фузер
  побеждает. Заменён на структурный field-index инвариант (`Mac==fields[1]`,
  `Ip==fields[2]`, `Hostname==fields[3]` + count). 72k execs / 0 крашей.
- **`9f2a447`** — P2.1 CSV-гарды использовали `check` (equality), не `>=`;
  здоровый CSV с >min строк ронял smoke (`2 != 7`). Добавлен `check_lines`
  (wc -l `>=`), 25/34 переведены на него.

## Расхождения с промтом (исправлено по ходу)

Полный разбор — в execution-логе §«Расхождения с промт-планом» и
§«Follow-up фиксы». Кратко: 7 из 11 задач содержали неточности —
P2.1 (counts уже вычислялись, только эхались), P2.2 (MAC/hostname/IP в
промте не совпадали с реальными), P2.7 (хелперы из примера не существуют),
P2.8 (`auditHandler` read уже покрыт), P2.9 (format-инвариант невозможен
для garbage-парсера), P2.10 (в примере фикса пропущен dialog-handler),
P2.11 (версии — timestamps, порядок уже newest-first, селектор промта
нерабочий). P2.3 и P2.4 — подтверждены дословно.

## Осталось (вне P2)

- **P3** (`predrel-test-remediation-p3.md`) — 9 polish-задач до v1.0.
- **Продуктовый security-аудит** — JWT alg-confusion, plugin trust,
  X-Forwarded-For, `hash, _ :=`.
- **A15** — KNOWN-CONDITIONAL на dnsmasq 2.80 (по решению оператора).
