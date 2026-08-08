# A15 Fix Report

Дата: 2026-08-08

## Причина

dnsmasq 2.80 отклонял `tag:guest` в `dhcp-host`, если этот тег не был
предварительно объявлен. Для статического хоста `tag:<name>` является условием
сопоставления, а не способом назначить тег хосту. Для назначения используется
`set:<name>`.

## Исправление

Коммит `8d3c84b`:

- host API и frontend принимают `set:<name>` и `id:<client-id>`;
- `tag:<name>` отклоняется для новых static-host записей;
- bulk JSON endpoint проверяет теги тем же правилом;
- smoke/E2E используют `set:iot,set:guest`;
- A15 удалён из `tests/known-bugs.txt` и smoke-проверок.

## Проверка

Compat-matrix с dnsmasq 2.80, 2.86 и 2.90 завершился без unexpected failures.
Актуальный прогон 2.90: `156/156`, `0 Fail`, `0 Known-fail`, `0 Skipped`.
Все три версии завершились с `smoke rc=0`.
