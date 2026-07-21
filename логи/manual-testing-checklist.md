# Чек-лист ручного тестирования Intermasq v3

Перед каждым прогоном: очистить `users.json`, удалить `/etc/dnsmasq.d/*.conf`,
сбросить `INTERMASQ_SECRET`. Это даст чистый setup-экран и предсказуемое
состояние dnsmasq.

## Подготовка тестового сервера

- [X] dnsmasq установлен и слушает DHCP/DNS на тестовом интерфейсе
- [X] `/etc/dnsmasq.d/` существует и доступен на запись пользователю,
      от имени которого запускается intermasq
- [X] `/var/lib/misc/dnsmasq.leases` доступен на чтение
- [X] `/proc/net/arp` доступен на чтение (default)
- [X] Если systemd: `systemctl status dnsmasq` отвечает без sudo
      (либо настроен `sudoers` для `systemctl restart dnsmasq`)
- [X] Сгенерирован `INTERMASQ_SECRET=$(openssl rand -hex 32)`
- [X] Браузер: Chrome/Firefox свежие, devtools открыт (`F4` / `F12` → Network,
      Console, Application → Local Storage)

## 0. Запуск и фатальные проверки

- [X] Запуск без `INTERMASQ_SECRET` → процесс падает с понятным `[FATAL]`
      сообщением, не стартует
- [X] Запуск с `INTERMASQ_SECRET` → `Intermasq v3.0 Started on :8081`
- [X] `curl -s http://localhost:8081/api/status` возвращает
      `{"setup_required":true, "version":"3.0", "dnsmasq_active":...}`
- [X] `/metrics` без auth → 401
- [X] `/swagger/index.html` открывается, список эндпоинтов актуален

---

## 1. Первичная настройка (setup)

- [x] Открыл `http://<ip>:8081` в браузере → виден setup-экран
- [x] Ввёл username=`admin`, password=`pass1234` → редирект на dashboard
- [x] Файл `/etc/intermasq/users.json` создан, содержит один bcrypt-хеш
- [x] Перезапустил процесс → setup-экран больше не появляется,
      сразу login (токен в localStorage)
- [x] Открыл второй браузер (incognito) → setup недоступен (403),
      только login

---

## 2. Аутентификация

- [ ] Login с неверным паролем → `invalid_credentials`, 401
- [ ] Login с неверным паролем 10 раз подряд → 429 `too_many_attempts`
      на 11-й (окно 1 минута, лимит 10)
- [ ] После 2 неудач + 1 успешный вход → следующий логин с тем же IP
      не упирается в лимит (rate-limit сброшен)
- [ ] Login с верным паролем → JWT в ответе, в localStorage
- [ ] Удалил token из localStorage → dashboard пропал, редирект на login
- [ ] Подсунул мусорный JWT (`Authorization: Bearer xxx.yyy.zzz`) →
      все API-запросы возвращают 401
- [ ] `curl -H "X-API-Key: $INTERMASQ_SECRET" http://localhost:8081/api/hosts`
      → 200 (api-key работает)
- [ ] Logout → текущий JWT больше не валиден (401 на следующем запросе),
      но свежий логин выдаёт новый токен

---

## 3. Статические хосты (вкладка Static)

### 3.1 Добавление

- [x] Форма: MAC=`aa:bb:cc:dd:ee:01`, IP=`10.0.0.11`, hostname=`test1` →
      хост добавлен, файл `10-static.conf` создан, в таблице виден
- [x] Заглянул в файл: `dhcp-host=aa:bb:cc:dd:ee:01,test1,10.0.0.11`
- [X] Тот же MAC ещё раз → `mac_duplicate` (409)
- [X] Тот же IP, другой MAC → `ip_duplicate` (409) с указанием конфликтующего
- [-] MAC=`00:00:00:00:00:00` → `invalid_data` (400) - применилось
- [-] MAC=`aa-bb-cc-dd-ee-02` (разделитель `-`) → принимается, сохраняется - ошибка перезапуска
      как есть
- [X] Без IP (только MAC + hostname) → сохраняется, в файле нет IP-токена
- [X] Без hostname (только MAC + IP) → сохраняется
- [X] С тегами `set:iot`, `tag:guest` → сохраняется,
      в файле `...,set:iot,tag:guest`
- [X] С невалидным тегом `xyz:foo` → `invalid_tag`
- [X] Hostname с точкой `dev.lan` → принимается (RFC 1123)
- [X] Hostname `_bad` → `invalid_data` (нижнее подчёркивание запрещено)

### 3.2 Редактирование

- [X] Клик на хост → форма заполняется, меняешь IP → save → запись обновлена
      (старая строка удалена, новая appended)
- [X] Сменил hostname на пустой → запись без hostname

### 3.3 Удаление

- [X] Trash-иконка на хосте → `confirm()` → запись удалена
- [X] Удаление несуществующего MAC → `host_not_found` (404)
- [X] Чекбоксы на 3 хостах → "Удалить выбранные" → все 3 удалены

### 3.4 Сортировка и поиск

- [X-] Клик по заголовку MAC → сортировка по MAC
- [X-] Клик по заголовку IP → умная сортировка IP (10.0.0.2 < 10.0.0.10)  
- баг, при сортировке копируются строки... можно так накопировать на всю страницу, но при обновлении они сьрасываются, это баг UI  
- пример  
- Онлайн	MAC ↕	IP ↕	Hostname ↕	Теги	Файл	Действия
	🟢	60:3d:61:28:89:5c	172.20.5.78	dwada23332	—	yadr00x05.conf	
	🟢	60:3d:61:28:89:5c	172.20.200.2	yandexST	—	iot172x20x200.conf	
	🟢	60:3d:61:28:89:5c	172.20.5.78	dwada23332	—	yadr00x05.conf	
	🟢	60:3d:61:28:89:5c	172.20.200.2	yandexST	—	iot172x20x200.conf	
	🟢	60:3d:61:28:89:5c	172.20.5.78	dwada23332	—	yadr00x05.conf	
	🟢	60:3d:61:28:89:5c	172.20.200.2	yandexST	—	iot172x20x200.conf	
	🟢	60:3d:61:28:89:5c	172.20.5.78	dwada23332	—	yadr00x05.conf	
	🟢	60:3d:61:28:89:5c	172.20.200.2	yandexST	—	iot172x20x200.conf	
	🔴	aa:bb:cc:dd:ee:03	10.0.0.18		—	00-test.conf	
	🔴	88:8e:68:0f:90:8c	172.20.0.10	wifiAPhuawey	—	skld172x20x0.conf	
	🟢	d8:44:89:42:28:c0	172.20.0.11	WifiTPlinkArcher	—	skld172x20x0.conf
- [X] Ввёл `test` в search → фильтр сработал по hostname

### 3.5 Bulk import / CSV 

- [X] "Bulk import" → вставил 3 строки `MAC IP Host` → все 3 добавлены  
испортирует, но каждый раз пишет что импортировано 0, хотя на деле все ок
- [X] Импорт CSV-файла → `bulk_add`, count=3
- [X] CSV с дубликатом внутри → `ip_duplicate_bulk` с mac1/mac2
- [X] CSV с дубликатом существующего → `ip_duplicate`
- [X] "Export CSV" → скачивается `intermasq_hosts.csv`, открывается в Excel в либре калк открылось  
заметка - неудобно. он вообще все экспортирует. надо добавить ещё возможность локального экспорта.

### 3.6 Bulk move

- [X] Создал второй файл `20-iot.conf` через Config-вкладку
- [X] Выбрал 2 хоста → "Move to 20-iot.conf" → хосты переехали
- [X] В исходном файле их нет, в целевом — есть
- [X-] Move на хост с конфликтом IP в целевом файле → skip,
      в ответе `skipped:[mac]` он мне не дает создать банально такую запись

### 3.7 Bulk edit

- [-] Выбрал 3 хоста → bulk-edit → old=`10.0.0`, new=`10.0.1` →
      все 3 IP переехали на новый префикс
- [ ] Bulk-edit с CIDR `10.0.0.0/24` → `10.0.5.0/24` → IP-адреса
      пересчитаны по маске - no_hosts
- [ ] Bulk-edit с mismatched форматами → `prefix_format_mismatch`
- [ ] Bulk-edit с hostname-suffix `-new` → все hostname получили суффикс  
Последние 3 не понял как сделать адекватно. edit - ничего не происходи но стоит мне убрать все галочки и появляются поля для заолнения, но там я постоянно получаю nohost


### 3.8 Шаблоны (Templates)

- [ ] Templates → "New" → name=`IoT`, pattern=`iot-{n}`,
      range=`10.0.5.0/24`, target=`20-iot.conf` → создан
- [ ] Apply на пустой MAC → выдался первый свободный IP + hostname
      `iot-1`
- [ ] Apply повторно → `iot-2`, следующий IP
- [ ] Delete template → из списка пропал  
У меня вообще по другому колонки name вторая выпадающая пустая, третья debice-{NNN} четвертая путь /etc/dsnmasq,d/hosts.conf 

---

## 4. DHCP-аренды и обнаружение (Discovery, Leases)

### 4.1 Leases

- [X] Открыл вкладку Leases → список текущих аренд dnsmasq
- [X] Подключил новое устройство в сеть → через 30с появилось в списке
- [X] Кнопка "To static" на одной аренде → хост создан, из списка аренд
   X  пропал (после применения)

### 4.2 ARP / Discovery

- [X] Статус `🟢` у онлайн-устройств (есть в ARP), `🔴` у оффлайн
- [X] Выключил устройство → через ~30с статус сменился на `🔴` (SSE пуш)
- [X] Включил обратно → `🟢`
- [ ] Discovered-devices: устройство в ARP, не в static, не в leases → 
      показано в списке "New devices" есть какие то, но я не знаю что это
- [нет таких ] Vendor показан (OUI-resolved) для известных вендоров
      (Apple, Samsung, и т.д.)
- [X ] Bulk lease→static на 3 устройства → все 3 в static,
      warning показан "нажмите Применить"

---

## 5. DNS Aliases (вкладка DNS)

### 5.1 A-записи

- [X] Type=`A`, domain=`nas.local`, target=`10.0.0.5` → добавлено
- [X] Заглянул в `10-dns-aliases.conf`:
      `address=/nas.local/10.0.0.5`
- [X] `dig @127.0.0.1 nas.local` после reload → `10.0.0.5`
- [ ] Duplicate domain+type → `alias_duplicate` (409) можно добавить полную копию
- [X] Target не IP → `invalid_data`

### 5.2 CNAME

- [X] Type=`CNAME`, domain=`www.local`, target=`nas.local` →
      `cname=www.local,nas.local`
- [X] `dig www.local` → CNAME → nas.local → 10.0.0.5

### 5.3 PTR / TXT

- [X] Type=`PTR`, domain=`5.0.0.10.in-addr.arpa`,
      target=`nas.local` → `ptr-record=...`
- [X] Type=`TXT`, domain=`_dmarc.local`, target=`v=DMARC1;...` →
      `txt-record=...`
- [X] TXT с переносом строки в target → `invalid_data`

### 5.4 CSV / bulk

- [X] Bulk add 3 записи (A + CNAME + TXT) одной кнопкой → все 3 добавлены
- [X] Export CSV → 3 строки + header
- [X] Import CSV обратно → все 3 на месте

### 5.5 Удаление

- [X] Trash на A-записи → строка удалена из файла, CNAME остался
- [X] Delete PTR через API (UI поддерживает только A/CNAME) →
      тип-фильтр `req.Type != "A" && req.Type != "CNAME"` → 400 
Нужно реализовать полноценное удаление
### 5.6 Default file

- [X] Первый add без указания файла → создан `10-dns-aliases.conf`
      с header-комментарием
- [X] Все последующие добавляются в этот же файл

---

## 6. Конфиг-редактор (вкладка Config)
Тут сложно у меня все пусто я ничего не трогал у меня нет файлов настроек надо грузить из шаблона и как то настраивать
### 6.1 Snapshot

- [X] Все `.conf` файлы показаны как табы
- [X] Директивы распарсены: key/value, active/inactive
- [X] `dhcp-range=` показан как range-структура с CIDR
- [X] Кнопка "🕒 История" активна, если у файла есть версии
- [X] Кнопка "⏪ Откат" активна, если есть `.bak`

### 6.2 Редактирование директив 

- [] Сменил value у `domain-needed` → save → файл перезаписан,
      `dnsmasq --test` OK
- [] Снял `active`-чекбокс → директива закомментирована `#...`
- [] Добавил новую директиву `port=5353` → сохранено
- [ ] Invalid value (например `port=abc`) → `dnsmasq_test_failed`,
      файл откатился к предыдущему состоянию   


### 6.3 dhcp-option / PXE / forwarding (если есть в шаблоне)

- [ ] dhcp-range отредактирован, save OK
- [ ] dhcp-option=tag:...,option:... переживает round-trip
- [ ] PXE-директива `dhcp-boot=...` сохраняется

### 6.4 Создание / удаление файла

- [ ] "New file" → name=`30-test.conf`, template=`empty` → создан,
      появился как таб
- [ ] Template=`basic-dhcp` → файл наполнен dhcp-range + опциями
- [ ] Template=`forwarder` / `pxe` / `aliases` → корректный контент
- [ ] name=`foo.txt` (не .conf) → `invalid_filename`
- [ ] name=`../etc/passwd` → `invalid_filename` или `access_denied`
- [ ] Delete file → двухступенчатый confirm → файл удалён,
      таб закрылся, `.bak` тоже зачищен
- [ ] Delete на `audit.log` или подобном (через прямой API call
      с `file=/etc/intermasq/audit.log`) → `invalid_filename` (не .conf)
- [ ] После delete → в History вкладки файл восстанавливается

### 6.5 Raw-режим

- [ ] "Edit as text" → текстовая версия файла
- [ ] Отредактировал вручную → save → test OK
- [ ] Вписал синтаксис с ошибкой `dhcp-host=` (пустой) →
      `dnsmasq_test_failed`, файл откатился

### 6.6 Path traversal (через curl)

- [ ] `curl -X PUT .../api/files/../../etc/passwd` → 403
- [ ] `curl ... /api/files/passwd` (no .conf) → 403
- [ ] `curl ... /api/history?file=/etc/shadow` → 400 `invalid_path`
- [ ] `curl -X POST .../api/history/restore -d '{"file":"/etc/shadow","version":"20240101-000000"}'`
      → 400 `invalid_path`

---

## 7. Safety / History

### 7.1 .bak rollback

- [X] Внёс изменение в хост → `.bak` создан (или обновлён)
- [X] Кнопка "⏪ Откат" → подтвердил → файл вернулся к предыдущему состоянию
- [X] Откат на файл без `.bak` → `rollback_error`

### 7.2 Multi-level history

- [X] Сделал 5 правок одного файла → в History 5 версий
- [X] Превысил `-history-depth` (10) → старые версии ротировались
      (осталось 10)
- [X] History list отсортирован newest-first
- [] Diff между v1 и v2 → показан unified diff (`-`/`+`)
- [ ] Diff между v1 и `current` → корректный
- [ ] Restore версии v2 → файл откатился, текущее состояние
      тоже попало в history (можно откатить откат)
- [ ] Restore версии с invalid-конфигом (подсунуть в файл мусор,
      сохранить как версию) → `dnsmasq_test_failed`,
      файл вернулся к pre-restore  
Это мне по ssh лезть и смотреть все с путями?

### 7.3 ZIP backup

- [X] "💾 Backup" → скачался `intermasq_backup_YYYY-MM-DD_HH-MM.zip`
- [X] Распаковал → все `.conf` файлы на месте
- [X] Внёс 3 правки → restore из ZIP → все правки откатились,
      создались `.restore.bak` файлы
- [X] Restore ZIP с мусором внутри → `dnsmasq_test_failed`,
      все изменения откатились, файлы не повредились
- [X] Restore ZIP с non-.conf файлами внутри → игнорируются

---

## 8. SSE (живые обновления)

- [X] Открыл dashboard, оставил на 5 минут → ARP-статусы
      пушатся без полного reload
- [ ] `tcpdump -i lo port 8081 and tcp` → видны SSE-фреймы
      `event: arp\ndata: ...` bash: tcpdump: command not found
- [ ] Остановил dnsmasq (`systemctl stop dnsmasq`) → badge сменился
      на 🔴 в течение 5 секунд
- [ ] Запустил обратно → 🟢
- [ ] Закрыл вкладку → сервер не копит disconnected clients
      (проверить `ss -tnp | grep 8081 | wc -l`)

---

## 9. Users (вкладка Users)

- [X] Список показывает текущего пользователя
- [X] Create user `alice` → 200, в списке появился
- [X] Create user `alice` ещё раз → `user_exists` (409)
- [X] Create с username length > 64 → `username_too_long`
- [X] Delete другого пользователя → 200
- [X] Delete самого себя → `cannot_delete_self` (400)
- [X] Change own password (wrong old) → `invalid_credentials`
- [X] Change own password (correct) → следующий логин с новым паролем OK
- [X] Logout из alice → её JWT в blacklist, повторно не используется
- [X] Параллельность: запустил 10 одновременных `curl -X POST /api/users`
      с разными именами → все 10 создались, `users.json` не повредился
      (можно проверить через `go test -race` локально)

---

## 10. Audit log

- [X] После каждого действия (add/delete/edit/reload/rollback) —
      запись в `/api/audit`
- [ X] Audit-вкладка: список отсортирован newest-last (UI делает reverse)
- [ x] Поле `user` заполнено корректно (или `api-key` для X-API-Key)
- [X] Разные действия имеют разные цвета (`config_delete_file` — красный,
      `user_create` — синий, и т.д.)
- [X] `password_change` логируется без самого пароля (только username)
- [X] Аудит-файл переживает рестарт процесса (append-mode)

---

## 11. /metrics 

- [ -] `curl http://localhost:8081/metrics` (без auth) → 401
- [ -] `curl -H "Authorization: Bearer <jwt>" /metrics` → 200,
      Prometheus-формат
- [ ] `curl -H "X-API-Key: $INTERMASQ_SECRET" /metrics` → 200
- [ ] `curl /metrics?token=<secret>` → 200 (для Prometheus scrape)
- [ ] Метрики присутствуют: `intermasq_hosts_total`, `_leases_active`,
      `_arp_online_total`, `_dnsmasq_active`, `_reloads_total`,
      `_dnsmasq_test_failures_total`, `_uptime_seconds`
- [ ] После 1 успешного reload → `intermasq_reloads_total` увеличился
- [ ] После 1 намеренно сломанного конфига → `_test_failures_total` +1
- [ ] При наличии DNS aliases → `intermasq_domain_up{domain=...}` и
      `_resolve_seconds{...}` появляются через 60с после старта
      (фоновый checker)  
получаю пустой вывод  
[root@SHLZ00 ~]# curl http://localhost:8081/metrics
[root@SHLZ00 ~]# curl http://localhost:8081/metrics

---

## 12. Перезапуск / init-system

### 12.1 Reload dnsmasq

- [X] "🔄 Применить" → `dnsmasq --test` OK → restart → успех
- [X] При сломанном конфиге → `reload_error`, dnsmasq не тронут
      (продолжает работать от старого)

### 12.2 Restart-self

- [X] "🔄 Restart" → подтвердил → 200, процесс перезапустился,
      через 5с страница сама reloadнулась
- [X] С `-ci-mode=true` → 200, но процесс НЕ перезапустился
      (для CI окружения)

### 12.3 Init-system определение

- [X] `-init-system=auto` на systemd-host → `[INIT] System: systemd (root)`
      или `(via sudo)`
- [X] `-init-system=systemd-user` → работает `systemctl --user restart dnsmasq`
- [X] `-init-system=none` → reload бесшумно succeeds (no-op),
      `RestartSelf()` возвращает ошибку
- [X] Legacy `-systemd-scope=user` → пишет warning, маппится на systemd-user

### 12.4 Portable binary paths позже

- [ ] На Alpine (где `dnsmasq` в `/usr/sbin`, а `sudo` в `/usr/bin`) →
      auto-resolve через `$PATH` находит оба
- [ ] `-dnsmasq-bin=/explicit/path` → используется указанный

---

## 13. Плагины (опционально, если есть готовый плагин) есть работает

- [ ] Создал `/etc/intermasq/plugins/test/manifest.json`:
      `{"id":"test","name":"Test","bin":"./test.sh"}`
- [ ] `test.sh` создаёт Unix-сокет из `$PLUGIN_SOCKET` env и отвечает
      "OK" на HTTP-запрос
- [ ] Старт intermasq → `[PLUGINS] Started Test on socket ...`
- [ ] `/api/plugins` → JSON со списком
- [ ] UI: в выпадающем меню "Плагины" появился Test
- [ ] Клик → открывается iframe на `/plugins/test/`
- [ ] Удалил каталог плагина, перезапустил intermasq → плагин пропал
      из списка и из UI

**Важно для ревью безопасности:** `INTERMASQ_KEY` передаётся плагину в env.
Плагин имеет полный admin-доступ через X-API-Key. Доверяй только своим плагинам.

---

## 14. i18n / UI

- [X] RU по умолчанию → русские подписи
- [X] 🌐 переключатель → EN, сохраняется в localStorage
- [X] F5 → выбранный язык сохранён
- [X] 🌓 тема → dark, body получает `data-bs-theme=dark`
- [X] F5 → тема сохранена
- [X] API-ошибки переведены: например, при `mac_duplicate` в RU —
      "MAC уже используется", в EN — "MAC address already exists"
- [X] Unknown error code → fallback на сам код ошибки

---

## 15. Path traversal / инъекции (фаззинг через curl) как это делать?

- [ ] `POST /api/hosts` с `file=/etc/passwd` → 400/403
- [ ] `DELETE /api/hosts/:mac?file=/etc/passwd` → 400/403
- [ ] `POST /api/aliases` с `file=../../../tmp/x.conf` → 403
- [ ] `GET /api/files/..%2F..%2Fetc%2Fpasswd` → 403
- [ ] `PUT /api/files/passwd` (non-.conf) → 403
- [ ] `POST /api/history/restore` с `file=/etc/hosts` → 400
- [ ] Hostname с управляющими символами `\n` → reject
- [ ] CSV-импорт с embed'нутым `"` → корректный parse (csv.Reader
      обрабатывает кавычки)

---

## 16. Граничные случаи

- [X] Пустой `.conf` файл → парсер не падает, snapshot пустой
- [X] Файл только с комментариями → в snapshot 0 директив
- [X] MAC в верхнем регистре `AA:BB:CC:DD:EE:FF` → сохраняется,
      дубликат через `aa:bb:...` детектится
- [X] Очень длинный hostname (64+ символов) → reject
- [X] IPv6 IP в dhcp-host (`fe80::1`) → парсится `net.ParseIP`
- [ ] Одновременная запись двух пользователей в один файл
      (race) → `mu` сериализует, файл не повреждается
- [X] Сетевое прерывание во время reload (выключить dnsmasq
      mid-request) → понятная ошибка, не hang

---

## 17. Производительность (smoke) не могу

- [ ] 200 хостов → `GET /api/hosts` < 500ms
- [ ] 200 хостов → таблица рендерится без лагов
- [ ] 50 SSE-клиентов одновременно → CPU не улетает в стратосферу
- [ ] 10 одновременных reload → dnsmasq не падает

---

## 18. Логи и диагностика ок 

- [ ] `[INIT] System: ...` пишется на старте
- [ ] `[PLUGINS] Started ...` для каждого плагина
- [ ] `[VALIDATION] MAC ... accepted/rejected` для каждого add
- [ ] `[HISTORY] write ... : <err>` если есть проблема с диском
- [ ] `tail -f /etc/intermasq/audit.log` → real-time поток событий

---

## 19. После тестов

- [ ] Снять `audit.log` и проверить покрытие: каждое действие
      оставило запись
- [ ] История в `/etc/intermasq/history/` — не разрослась
      сверх `history-depth * число_файлов`
- [ ] `.bak`-файлы не копятся бесконечно (один на файл)
- [ ] `.restore.bak` после ZIP-restore удалены (или явно оставить
      — это ожидаемо, документировать в README)
- [ ] `go test -race ./...` локально перед релизом

---

## Известные инерции (не баги, но иметь в виду)

- Версия в `/api/status` захардкожена `"3.0"`, ldflag `main.version`
  не подключён (см. `планов/дальнейшее.md:36`).
- Глобальный `mu` сериализует все writes в `.conf` — узкое место,
  но для 1-2 админов дома неактуально.
- `/metrics` использует gauge-counter'ы вместо настоящих `counter`
  (promauto). `rate()` в Prometheus работает, но semantically не quite.
- JWT blacklist в памяти — после рестарта все logout'ы invalidated.
- Плагины не supervised: упал → не поднимется, UI будет 502.
