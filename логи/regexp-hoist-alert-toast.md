# Сессия: regexp hoisting + alert→toast

**Дата:** 23 июля 2026
**Ветка:** `main`
**Коммитов:** 1 (10684dd)

## Контекст

Два code-quality замечания от внешнего ревьюера:
1. Все ошибки в UI показываются через `alert()` — блокирует поток
   браузера, нет toast/snackbar.
2. `regexp.MustCompile` вызывается на каждый HTTP-запрос вместо
   package-level компиляции.

Оба замечания подтвердились проверкой кода.

## Что было сделано

### Backend — regexp hoisting (3 файла)

| Файл | Было | Стало |
|---|---|---|
| `handlers_config.go:33` | `directiveKeyValidator := regexp.MustCompile(...)` на каждый запрос | `directiveKeyValidator := directiveKeyRegex` (использует существующий package-level регекс из `config_snapshot.go:39`) |
| `config_snapshot.go:203` | `leaseRe := regexp.MustCompile(...)` внутри `isLeaseTime()` | `leaseTimeRegex` — package-level var, используется напрямую |
| `dnsmasq.go:227` | `octetRe := regexp.MustCompile(...)` внутри `parseIPTransform()` | `octetPrefixRegex` — package-level var |

Особенность `handlers_config.go`: handler пересоздавал **точную копию**
регекса, который уже существовал как `directiveKeyRegex` в
`config_snapshot.go:39`. Дубликат был не просто неэффективен, но и
поддерживал риск рассинхронизации паттернов.

### Frontend — alert → toast (10 файлов)

**Новые файлы:**
- `frontend/src/toast.js` — reactive composable на Vue 3 `reactive()`.
  API: `toast.success(msg)`, `toast.error(msg)`, `toast.warning(msg)`,
  `toast.info(msg)`. Auto-dismiss: success 5с, error 8с, warning 6с.
- `frontend/src/components/ToastContainer.vue` — рендерит Bootstrap 5
  `.toast-container` (position-fixed top-right, z-index 9999). Иконки
  по типу (✅/⚠️/⚡/ℹ️), кнопка закрытия.

**Изменено (App.vue + 8 компонентов):**
Все 23 вызова `alert()` заменены на `toast.success()` / `toast.error()`:

| Компонент | Было alert() | Стало |
|---|---|---|
| HostForm.vue | 7 (validation + catch + import success) | toast.error / toast.success |
| DnsmasqConfig.vue | 4 (save/rollback/delete success + catch) | toast.error / toast.success |
| AliasForm.vue | 4 (validation + add error + import success) | toast.error / toast.success |
| HostTable.vue | 2 (catch + bulk delete error) | toast.error |
| StaticView.vue | 2 (rollback success + catch) | toast.error / toast.success |
| DnsAliasesView.vue | 2 (rollback success + catch) | toast.error / toast.success |
| DiscoveredTab.vue | 1 (no target file) | toast.error |
| LeasesTab.vue | 1 (no target file) | toast.error |

Проверка: `grep -r "alert(" frontend/src/` → 0 совпадений.

## Результат

```
go test:         PASS (241 тест)
go vet:          OK
gofmt:           clean
npm run build:   ✓ built in 2.21s
Pipeline:        зелёный
```
