# Gap 2 — Playwright E2E, батч 3 фаза В (UI coverage, +5 tests / 4 specs)

**Дата:** 2026-07-25
**Gap:** 2 (L4 — UI/E2E), продолжение `gap2-batch3-phaseB.md`
**Коммиты:** `acd1bfb` (4 specs), `5dc01d5` + `48b9532` (2 фикса путей в хелпере)
**Результат:** CI `run_e2e_tests=true` — **25/25 passed**

## Контекст

Фаза В — самая сложная часть батча 3: rollback/history (нужны `.bak` и
versioned-снапшоты), discovery (arp-fixture), backup/restore (blob-download
+ dynamic filechooser + реальный zip). По плану
(`C:\Users\alexr\AppData\Local\Temp\opencode\l4-batch3-plan.md`).

## Что сделано (+5 tests → 25)

| spec | что проверяет | нюансы |
|---|---|---|
| `rollback-ui` | 2 seed'а в файл → `.bak`=1-хост состояние → выбрать таб → ⏪+confirm → 2-й хост исчез | `rollbackFile` revert из `.bak` |
| `history-modal` | 2 seed'а → 🕒 → версия в списке → ≠ diff (`pre.history-diff`) → ⏪ restore+confirm → 2-й хост исчез | version1 = snapshot 1-хост состояния |
| `discovery-tab` | 🔍 → MAC из arp-fixture (`11:22:33:44:55:01`) в newDevices → ➕ → MAC перетёк в HostForm | leases пустой → только newDevices-секция |
| `backup-restore-ui` (download) | 💾 → blob-download, `suggestedFilename='dnsmasq_backup.zip'` | `waitForEvent('download')` |
| `backup-restore-ui` (restore) | 📤 → `filechooser` → реальный zip (из API `/backup` в beforeAll) → confirm → удалённый хост вернулся | `restoreBackupZip` MERGES (не wipe) → другие спеки не страдают |

**Ключевая разведка:** `createLocalBackup` (history.go:259) зовёт `saveHistory`
+ пишет `.bak` из текущего состояния. Поэтому `.bak` и version создаются
**только при 2-м write** в файл (1-й write на пустом файле → no-op). Отсюда
паттерн «сею 2 хоста в один файл» в rollback/history-спеках.

## Грабли (два красных CI-прогона до зелёного)

Оба — одна и та же болезнь: **Playwright APIRequestContext использует host-only
baseURL, поэтому все пути требуют `/api`-префикс**. `seedHosts`/`apiLogin` были
корректны, а:

1. `backup-restore-ui` звал `ctx.get('/backup')` → 404. Фикс: `/api/backup`
   (`5dc01d5`).
2. `deleteHostApi` в `lib/api-hosts.ts` звал `ctx.delete('/hosts/${mac}')` →
   404 → silent no-op → R1 не удалялся → precondition restore-теста
   `toHaveCount(0)` валился. Фикс: `/api/hosts/${mac}` (`48b9532`).

**Интересный момент по качеству:** `deleteHostApi` содержал неправильный путь
ещё с фазы А, но ни один спек его не дёргал (остальные удаляют через UI) —
поэтому баг спал, пока backup-restore-ui не вызвал его в beforeAll. Это
хороший аргумент за разовый **mutation/usage-pass**: «всё зелено» не значит
«всё проверено» — мёртвый-с-ошибкой код вскрылся только тогда, когда его
наконец вызвала.

## Результат

CI (Forgeora, `fedora:44`): **25/25 passed**. Батч 3 закрыт — цель ~24 specs
достигнута и перевыполнена. bulk-edit остаётся `test.fail` (A5).

## Где мы по L4 теперь

25 specs, 21 файл. Основной UI (auth/theme/i18n/CRUD/sort/search/tags/
bulk/config/templates/users/dns/import/reload/rollback/history/discovery/
backup-restore) под прикрытием. A1 — guard, A5 — точный репродюсер.

## Что осталось

- **Батч 4** (6 нишевых + 2 жёстких): audit-tab, plugins-iframe,
  i18n-api-error, config-template-fill, setup-screen, +sse-live (мутация
  arp-файла), +config-directive/raw (блок A13).
- **true A5-фикс** (1 строка) → снять `test.fail`.
- **(опционально) mutation-pass** — намеренно сломать 5 строк продукта,
  прогнать CI, убедиться что соответствующие спеки краснеют. Эмпирическая
  уверенность, что тесты реально ловят.

## Локальная проверка

- `npx playwright test --list` — **25 tests in 21 files**.
