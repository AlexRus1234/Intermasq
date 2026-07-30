# Coverage sweep — session log

Промт: `логи/Coverage_sweep.md`. Замеры — локально (Windows),
`$env:INTERMASQ_SECRET="ci-test-secret-32-bytes-pad-XXXXXXXXXX"`.

## Старт

| Замер | Значение |
|---|---|
| `go test ./... -cover` (базовый) | **66.0%** |

## Категория A — pure unit

**Цель:** закрыть pure-функции из §2.A 카드ою. **DoD:** ≥90% на каждую.

### Файлы

| Файл | Тип | Что добавлено |
|---|---|---|
| `bins_test.go` (новый) | L1 | `TestResolveBin*` (5 кейсов — empty/dir/nonexist/flag-exec/fallback/candidate), `TestIsExecutable` (nonexist/dir/noexec/exec), `TestLazyAccessors_CallResolve` (6 аксессоров) |
| `metrics_test.go` (новый) | L1 | `TestBoolToFloat`, `TestWriteSimpleMetric`, `TestWriteLabeledMetric` (+multi-labels), `TestCheckMetricsAuth*` (7 кейсов: APIKey ok/wrong, ?token= secret/jwt/invalid, Bearer ok/invalid, no-auth) |
| `templates_test.go` (новый) | L1 | `TestLoad Templates_NoFile`, `_ValidJSON`, `TestSaveTemplates_RoundTripAndAtomic`, `_MkdirAllError`, `_WriteError`, `TestGenHostnameFromPattern`, `TestCountHostsInFile` |
| `dnsmasq_test.go` (append) | L1 | `TestParseIPTransform` (17 кейсов — все err-ветви + octet/cidr/none), `TestIPTransform_Apply_None/_InvalidIP/_Octets/_CIDR/_CIDRRoundTrip`, `TestEnsureAliasesFile` (traversal/new-file/exists), `TestIsLeaseTime` (13 кейсов), `TestDirectiveGroup` (все 4 группы) |
| `handlers_test.go` (append) | L1 | `TestCoalesce`, `TestValidateHostTags` (11 кейсов), `TestNormalizeHostTags` (7 кейсов — trim/lower/dedup/order) |

### Результаты по функциям (`go tool cover -func`)

| Функция | Было | Стало | Примечание |
|---|---|---|---|
| `ensureAliasesFile` | 0% | **100%** | все 3 ветви |
| `isExecutable` | 75% | **100%** | +exec-bit ветвь на Linux |
| `parseIPTransform` | 30.4% | **100%** | все err-кейсы |
| `ipTransform.apply` | 39.1% | **91.3%** | residual = Windows-пропуск exec-CIDR; на Linux 100% |
| `validateHostTags` | 28.6% | **100%** | |
| `normalizeHostTags` | 44.4% | **100%** | |
| `coalesce` | 0% | **100%** | |
| `checkMetricsAuth` | 46.7% | **100%** | все 5 auth-способов |
| `boolToFloat` | 66.7% | **100%** | |
| `writeLabeledMetric` | 0% | **100%** | |
| `writeSimpleMetric` | 100% | **100%** | (был уже) |
| `isLeaseTime` | 60% | **100%** | |
| `directiveGroup` | 80% | **100%** | все 4 группы |
| `loadTemplates` | 60% | **80%** | residual = 2 `os.Exit` (вне обл. §6) |
| `saveTemplates` | 75% | **100%** | +MkdirAll/Write error-ветви |
| `genHostnameFromPattern` | — | **100%** | бонус |
| `countHostsInFile` | — | **100%** | бонус |
| `resolveBin` | 63.6% | **81.8%** | residual = Windows-пропуск exec-бита; на Linux ~100% |
| `dnsmasqBin/sudoBin/...` аксессоры | 0% | **66-100%** | lazy-init через `sync.Once` |

### Δ coverage

```
Базовый:  66.0%
После A:  69.5%    (+3.5%)
```

### Верификация

- `gofmt -l` — пусто (после `gofmt -w`)
- `go vet ./...` — чисто
- `go test ./... -count=1` — зеленый
- существующие тесты не сломаны

### Замечания / observability

- `loadTemplates` 80% < A-DoD 90%, но residual — две `os.Exit`-ветки (read-error,
  unmarshal-error), что в §6 явно отнесено к «вне области / ~100%». На Linux CI
  ситуация не улучшится (нужен subprocess-тест). Считаем T-A выполненным с
  оговоркой.
- `resolveBin` 81.8% локально < 90%, но Windows-специфично (не honoring exec-bit).
  На CI Linux закроется полностью — добавлены `runtime.GOOS=="windows"` skip'ы
  для FlagExecutable/FallbackExecutable/CandidateFound, которые там пробегут.
- `apply` 91.3% локально — резидуум тоже Windows-only. На Linux 100%.

### Что дальше

- Блок B (Linux+dnsmasq success-ветки через fake-dnsmasq скрипт)
- Блок C (рефакторинг bootstrap/горутин)
- Блок D (vanity — только если оператор попросит цифру >90%)

`tests/ROADMAP.md` чекбокс «≥70%» не тикнут: локально 69.5%. Ожидается, что
CI Linux (где бегут Linux-gated тесты блока B после его реализации) перешагнёт
70%. Тикнём по достижении.