# P1 — критичные починки тестовой инфраструктуры ✓ ВЫПОЛНЕНО

**Статус:** ✓ ВЫПОЛНЕНО (2026-08-02, коммит `7a84a2e`, push в `main`).
**Подробный лог:** [`predrel-test-remediation-p1-exec.md`](./predrel-test-remediation-p1-exec.md).

---

## Сводка по задачам

| Задача | Статус | Кратко |
|--------|:------:|--------|
| **P1.1** | ✓ | `known-bugs.txt` / `bugs.md` / `ROADMAP.md` синхронизированы (A14 FIXED, A15 KNOWN-CONDITIONAL на dnsmasq 2.80); `バグ` → `bug` в `51-history-diff-restore.sh:35` |
| **P1.2** | ✓ | `check()` расширен 5-м аргументом `body_pattern`; тег A14 снят с `52-backup-restore.sh`; A15 + `body_pattern 'dnsmasq_test_failed'` |
| **P1.3** | ✓ | 4 ноль-ассерт теста: `TestSystemdCallerRestartSelf` удалён; `TestSseBroadcastFullChannel` / `TestLazyAccessors_CallResolve` / `TestCleanupBlacklistOnce_EmptyMap` усилены реальными ассертами |
| **P1.4** | ✓ | `fakeDnsmasqArgvInspect` + `readArgvLog` хелперы + 3 wiring-теста (`_PassesConfFileToTest` для PutFile/UpdateConfig/RestoreBackup) как A13/A14 regression guards |
| **P1.5** | ✓ | `hosts-sort.spec.ts` seed де-коррелирован (IP и hostname в противо-направлениях); 5 ORDER-ассертов пересчитаны; мутация `sortKey.value = key` ловится |
| **P1.6** | ✓ | `go vet`/`build`/`test ./...` зелёные; `bash -n` чистый; rg-инспекции (`バг`, A14 в known-bugs) пусты |

## Дополнительно (по решению оператора)

**Продуктовый фикс A14** — `backup.go:119` `restoreBackupZip`: bare `dnsmasq --test`
заменён на per-file `--conf-file=` loop (вариант A из 3 рассмотренных). Тот же
канонический паттерн, что A13 в `dnsmasq.go:77,97` / `history.go:245`. Префикс
`dnsmasq_test_failed:` сохранён. Ни один из 12 `TestRestoreBackup*` не упал.

## Расхождения с промтом (исправлено по ходу)

Полный разбор — в execution-логе §«Расхождения с промт-планом». Кратко:

- ROADMAP «пуст»: фактические строки **51, 180, 192** (не 193; на 180 — дважды).
- `backup.go:119` **НЕ** передаёт `--conf-file=` (это и есть A14, не фикс).
- Хелпер `withDnsmasqBin` **не существует** — wiring через `dnsmasqBinPath`/`setBinPath`.
- Логика сортировки в **`HostTable.vue:82-126`**, не в `store.js`.
- `accessors` переменной в `bins_test.go:162` нет (inline slice literal).
- `check` для A15 на строке **37**, не 38.
- P1.3.c: `if got == ""` сломало бы тест на хостах без бинарника → заменено на идемпотентность/cache-consistency.

## Осталось (вне P1)

- **A15** — KNOWN-CONDITIONAL на dnsmasq 2.80 (по решению оператора).
- ~~**P2** (`predrel-test-remediation-p2.md`) — 11 задач перед рефакторингом init/backup/metrics/audit/sse.~~ → ✓ ЗАКРЫТО 2026-08-03 (см. `predrel-test-remediation-p2-exec.md`).
- **P3** (`predrel-test-remediation-p3.md`) — 9 polish-задач до v1.0.
- **Продуктовый security-аудит** — JWT alg-confusion, plugin trust, X-Forwarded-For, `hash, _ :=`.
