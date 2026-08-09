# Security Policy

## Reporting a vulnerability

Please do not report security vulnerabilities through public issues, pull
requests, or other public discussions.

Use the private vulnerability-reporting feature of the Forgejo instance that
hosts this repository, if it is available. You can also contact the maintainer
privately through:

- Matrix: `@alexrus1234:matrix.alexrus1234.ru`
- Telegram: `AlexRus1234`
- Email: `alexrus1234alex@gmail.com`

Do not include secrets, production credentials, or personal data in the initial
report.

Please include, when possible:

- a short description of the vulnerability and its impact;
- the affected version, commit, or deployment configuration;
- clear reproduction steps or a minimal proof of concept;
- relevant logs, HTTP requests, or screenshots with credentials and personal
  data removed;
- any suggested mitigation or fix.

Maintainers will acknowledge a report as soon as practical, investigate it,
and coordinate disclosure with the reporter. Please allow time for a fix and
an announcement before disclosing the issue publicly.

## Scope

Security reports are especially relevant to:

- authentication, JWT, API-key handling, RBAC, and rate limiting;
- unauthorized access to the web panel, API, plugins, metrics, or SSE
  endpoints;
- path traversal or unauthorized file access through configuration, history,
  backup, or restore operations;
- command injection, privilege escalation, or unsafe interaction with
  `dnsmasq` and system services;
- exposure of secrets, user data, audit logs, or configuration files.

Reports about unsupported or intentionally exposed services, weak passwords,
or insecure deployment configuration should clearly distinguish the deployment
issue from a vulnerability in Intermasq itself.

## Supported versions

There is currently no separately published supported-version matrix. Unless a
release specifies otherwise, security fixes are made against the current
development branch. Users should keep their checkout up to date and run
Intermasq only with a properly protected `INTERMASQ_SECRET`, restricted network
access, and appropriate filesystem permissions.

## Disclosure

Please keep vulnerability details private until the maintainers and reporter
agree that coordinated disclosure is appropriate. Once fixed, the project may
publish a short advisory or changelog entry describing the impact and affected
versions without exposing sensitive report details.

---

# Политика безопасности

## Сообщение об уязвимости

Не публикуйте сведения об уязвимостях в открытых issues, pull request или
других публичных обсуждениях.

Если это возможно, используйте приватный механизм сообщений об уязвимостях
на Forgejo-инстансе, где размещён репозиторий. Также можно связаться с
мейнтейнером напрямую через приватный канал:

- Matrix: `@alexrus1234:matrix.alexrus1234.ru`
- Telegram: `AlexRus1234`
- Email: `alexrus1234alex@gmail.com`

Не прикладывайте к первому сообщению секреты, рабочие учётные данные или
персональные данные.

По возможности укажите:

- краткое описание уязвимости и её последствий;
- затронутую версию, commit или конфигурацию развёртывания;
- точные шаги воспроизведения или минимальный proof of concept;
- относящиеся к проблеме логи, HTTP-запросы или скриншоты без секретов и
  персональных данных;
- предлагаемое исправление или способ временного снижения риска.

Мейнтейнеры подтвердят получение сообщения, изучат проблему и согласуют с
автором порядок раскрытия. Дождитесь исправления и объявления до публичного
раскрытия деталей.

## Область действия

Особенно важны сообщения об аутентификации, JWT, API-ключах, RBAC, ограничении
частоты запросов, доступе к файлам и конфигурациям, path traversal, command
injection, повышении привилегий, взаимодействии с `dnsmasq` и системными
службами, а также утечках секретов, данных пользователей, audit-логов и
конфигурационных файлов.

## Поддерживаемые версии

Отдельная матрица поддерживаемых версий пока не опубликована. Если в описании
релиза не указано иное, исправления безопасности вносятся в текущую ветку
разработки. Используйте защищённый `INTERMASQ_SECRET`, ограничивайте сетевой
доступ и корректно настраивайте права файловой системы.

## Раскрытие информации

До согласования с мейнтейнерами и автором сохраняйте детали уязвимости в
тайне. После исправления проект может опубликовать краткое security advisory
или запись в истории версий без раскрытия чувствительных деталей сообщения.
