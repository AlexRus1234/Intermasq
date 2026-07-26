# Gap 2 — финал, Блок A: фиксы A5 + A13

**Дата:** 2026-07-26
**Gap:** 2 (L4 — UI/E2E), префикс к `Gap_2_finish.md` (§5, Блок A)
**Коммиты:** `7cd0e1d` (A5 + A13 + smoke/A13-чек), `949ae4f` (изоляция A3/A4-хостов)
**Результат:** CI default — smoke.sh зелёный (Pass 129 / Fail 0 / Known-fail 9); `go test -race` зелёный; `npx playwright test --list` — 25 тестов (bulk-edit без `.fail`).

## Контекст

`Gap_2_finish.md` делит финал L4 на 3 блока: A (фиксы A5/A13), B (батч 4 — 8
спеков), C (mutation-pass). Эта сессия — только Блок A: продуктовые фиксы в
строго ограниченном объёме + необходимые правки smoke под новый behavior.
Блок B/C — отдельно.

## Что сделано

### A5 (HIGH, frontend) — BulkEditModal крашится при открытии

**Реальный root cause** (пин из L4-репродюсера `bulk-ops.spec`, а НЕ гипотеза
из `bugreport/bugs.md`): `preview` computed в `BulkEditModal.vue:67` звал
`store_hosts.find(...)` — а `store_hosts` это reactive-объект `store`, у
которого **нет** `.find`. TypeError при рендере → модалка не открывается.

**Фикс (1 строка):**
```js
- const host = store_hosts.find(x => x.mac === h.mac)
+ const host = store_hosts.hosts.find(x => x.mac === h.mac)
```

`bulk-ops.spec.ts`: снят `test.fail` с bulk-edit-теста, убраны A5-комментарии.

### A13 (HIGH, backend) — `dnsmasq --test` без `--conf-file=<path>`

`writeFileRaw` / `writeConfigWithTest` / `restoreHistoryVersion` гоняли bare
`dnsmasq --test` — он тестил **default config** dnsmasq (`/etc/dnsmasq.conf` +
`conf-dir=/etc/dnsmasq.d`), а не только что записанный файл. Невалидный
синтаксис (`port=abc`) проходил проверку и валил dnsmasq на следующем reload'е.

**Фикс (3 строки, канонический паттерн из `dnsmasq_test.go:1882`):**
```go
- testCmd := exec.Command(dnsmasqBin(), "--test")
+ testCmd := exec.Command(dnsmasqBin(), "--test", "--conf-file="+path)
```
в трёх местах: `dnsmasq.go:77` (`writeFileRaw`), `dnsmasq.go:97`
(`writeConfigWithTest`), `history.go:245` (`restoreHistoryVersion`, там
переменная `filePath`).

**НЕ трогал** (по §5): `sse.go:110` (`reloadDnsmasq`) и `backup.go:119`
(`restoreBackupZip`) — они тестируют конфиг-дир в целом, смена флага меняет
семантику reload/restore. Это отдельная задача.

**Снято:** `A13` из `tests/known-bugs.txt`; smoke-чек `40-config-files.sh:36`
(«PUT with invalid dnsmasq syntax → 400») — был KNOWN-fail, стал ожидать
реальный 400 (теперь честный). Комментарии в `41/51/52` обновлены под новый
behavior.

## Грабли (один красный CI-прогон до зелёного)

После A13-фикса smoke упал ровно на 1 check: `51-history-diff-restore.sh:32`
«Restore known version → 200» получал **500** вместо 200.

**Корень:** `restoreHistoryVersion` теперь честно валидирует восстанавливаемый
файл через `--conf-file=<path>`. А `$FILE` (`/tmp/conf/10-static.conf`, который
реставрит suite 51) был **отравлен** баг-регрессиями A3/A4 из
`21-hosts-bugs.sh`: zero-MAC `00:00:00:00:00:00`, broadcast `ff:ff:ff:ff:ff:ff`,
dash-MAC `aa-bb-cc-dd-ee-07`. Host-add пишет их через отдельный writer **без**
`dnsmasq --test`, поэтому они молча сохранялись во все снапшоты `10-static.conf`
(раньше restore не замечал — bare `--test` валидил дефолт-конфиг; теперь режектит).

**Фикс (только smoke, `949ae4f`):** A3/A4-хосты пишутся в отдельный
`19-bugs.conf` вместо `10-static.conf`. Сами A3/A4-проверки не изменились (всё
ещё KNOWN-fail — баги MAC-валидации не чинены), но `10-static.conf` теперь
содержит только валидные dhcp-host → `restoreHistoryVersion` проходит → 200.

**Важное наблюдение:** это **не слабость A13-фикса**, а вскрытый им
test-design-дефект — bug-regression-сьют отравлял файл, который использует
history/restore-сьют. Изоляция — правильное разделение ответственности.

## Верификация

Локально (Windows, продукт-код):
- `go vet ./...`, `gofmt -l`, `go build ./...` — чисто.
- `go test ./... -race -count=1` (`INTERMASQ_SECRET` задан) — **ok** 77.3с.
  A13-фикс не сломал `TestWriteFileRaw*` (тест игнорирует ошибку и проверяет
  только `.bak`, который пишется до `--test`).
- `npx playwright test --list` — 25 тестов / 21 файл, bulk-edit теперь обычный `test`.

CI (Forgejo, `fedora:44`, default run): smoke.sh **зелёный** —
Pass 129 / Fail 0 / Known-fail 9 (A2/A3/A4/A6/A8/A11/A12). L4 (opt-in
`run_e2e_tests=true`) не гонялся в этой сессии — bulk-edit без `.fail`
подтвердится в следующем e2e-прогоне.

## Где мы по L4 теперь

25 specs, 21 файл. A5 **FIXED**, A13 **FIXED**. Остаток по `Gap_2_finish.md`:
батч 4 (§6.1–6.8) + mutation-pass (Блок C) + финальный session-лог.

## Что осталось (вне этой сессии)

- **Блок B** — батч 4: audit-tab / plugins-iframe / i18n-api-error /
  config-template-fill (низкий риск); config-directive / config-raw
  (разблокированы фиксом A13); setup-screen / sse-live (HIGH, infra-решение).
- **Блок C** — mutation-pass на throwaway-ветке (4–5 мутаций).
- **Финальный session-лог** `Gap_2_finish.md` (весь путь) + ROADMAP L4 → финал.
