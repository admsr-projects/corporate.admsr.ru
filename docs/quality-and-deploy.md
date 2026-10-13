# Проверки, тестирование и развёртывание

## Какие проверки вообще есть

| Проверка | Команда | Статус |
|----------|---------|--------|
| Сборка фронтенда | `npm run build` | ✅ |
| Компиляция Go | `cd backend && go build ./...` | ✅ |
| Статический анализ Go | `cd backend && go vet ./...` | ✅ |
| Тесты Go | `cd backend && go test ./...` | ✅ 41 тест-функция в `internal/handlers`, `internal/courses`, `internal/tests`, `internal/media` и `internal/profanity` |
| Smoke схемы курсов | `npm run test:courses` | требует PHP + доступ к БД |
| Проверка типов TS | `npm run typecheck` | ⚠️ vue-tsc с базовой линией: падает, только если ошибок стало больше, чем в `scripts/typecheck-baseline.json` (сейчас 113; ADR-045) |
| Линт | — | ❌ не настроен |
| Тесты фронтенда | — | ❌ отсутствуют (`npm test` — заглушка) |
| CI | — | ❌ отсутствует |

**[ПОДТВЕРЖДЕНО]**

### Выполнено при составлении этой документации (2026-09-22)

```
cd backend && go vet ./...     → без замечаний
cd backend && go build ./...   → успешно
cd backend && go test ./...    → ok corporate.admsr.ru/backend/internal/handlers (9 PASS)
```

`npm run build` **не запускался**, чтобы не переписывать `dist/`,
`components.d.ts` и `auto-imports.d.ts` в рабочей копии во время активного
редактирования. Утверждения о фронтенде в этой документации получены **чтением
кода**, а не запуском.

## Тесты Go

`backend/internal/handlers/`:

| Файл | Что покрывает |
|------|---------------|
| `auth_guards_test.go` | `TestEndpointsRequireSessionBeforeTouchingDB` (в т.ч. подделка `X-User-Id`), `TestSyncRequiresSharedSecret`, `TestSyncRejectsWhenSecretNotConfigured`, `TestTestsModuleIgnoresClientUserID`, `TestAbsenceJournalRequiresAuth` |
| `session_bootstrap_test.go` | `TestSessionBootstrapRejectsBodyIDWithoutSession`, `TestSessionBootstrapRejectsNonPost` |
| `news_cursor_test.go` | курсорная пагинация ленты |

Приём, принятый в этих тестах: хендлер вызывается с **nil-пулом БД**. Если guard
отработал — вернётся 401/403; если guard пропущен, код дойдёт до запроса в БД и
упадёт паникой. То есть тест падает на «старом» (уязвимом) коде и проходит на
исправленном. **[ПОДТВЕРЖДЕНО — `PROJECT_AUDIT.md`, SEC-001]**

Новые guard'ы стоит покрывать по тому же образцу.

## Ручная проверка API

```bash
curl -fsS -H "Host: corporate.admsr.ru" http://127.0.0.1/api/health.php
```

Ожидается `"backend":"go"` — значит запрос дошёл до Go, а не до PHP.

Проверка закрытости эндпоинта (должно быть 401):

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/users.php
```

## Развёртывание

Целевая ОС: Ubuntu 20.04 / 22.04. Полная инструкция первого развёртывания — в
[`/README.md`](../README.md), раздел «Развёртывание». Здесь — то, что нужно знать
при внесении изменений.

### Состав

| Компонент | Файл |
|-----------|------|
| Скрипт обновления | `deploy/deploy.sh` (498 строк) |
| Переменные | `deploy/deploy.env` (из `deploy.env.example`, в `.gitignore`) |
| systemd-юнит Go API | `deploy/corporate-go-api.service` |
| nginx: основной хост | `deploy/nginx-corporate.admsr.ru.conf` |
| nginx: allowlist волны 2 | `deploy/nginx-go-api-wave2.conf` (подключается через `include`) |

### Что делает `deploy.sh`

1. Бэкап PostgreSQL (`pg_dump`), если задан `PGPASSWORD`; хранит `BACKUP_KEEP=7`.
2. Сброс автогенерируемых `auto-imports.d.ts`, `components.d.ts`, `src/route-map.d.ts`.
3. `git pull --ff-only origin <GIT_BRANCH>`.
4. `npm ci` (или `pnpm install --frozen-lockfile`).
5. `npm run build`.
6. Каталоги данных.
6b. Миграции: `psql -v ON_ERROR_STOP=1 -f` по всем `db/migration/V*.sql` — только
   при заданном `PGPASSWORD` и наличии `psql`.
6c. Go toolchain: при отсутствии `go` скачивает версию `GO_VERSION`
   (при `GO_AUTO_INSTALL=1`).
6d. Проверка `backend/.env`.
6e. `CGO_ENABLED=0 go build -o bin/api ./cmd/api`, `systemctl daemon-reload`,
   `enable`, `restart corporate-go-api`.
7. `systemctl reload php8.3-fpm`.
8. Health-check `GET /api/health.php` с ретраями (`HEALTH_RETRIES=10`,
   `HEALTH_SLEEP=2`).

Флаги: `-b <branch>`, `-n` (dry-run), `--skip-backup`, `--skip-pull`,
`--skip-go`; переменная `GO_SYNC_NGINX=1` дополнительно копирует конфиг nginx
из репозитория.

> **nginx без `GO_SYNC_NGINX=1` не перезагружается.** `nginx-go-api-wave2.conf`
> подключён `include`'ом из каталога приложения, и `git pull` его обновляет, но
> новые `location =` начинают действовать только после `sudo nginx -t && sudo
> systemctl reload nginx`. Иначе новый Go-эндпоинт уходит в PHP и отдаёт 404
> «File not found.» — см. IMP-63 в [known-issues.md](known-issues.md).

> **Версия Go.** `deploy.sh:22` по умолчанию ставит `GO_VERSION=1.26.5` — в паре
> с `backend/go.mod`. Меняя одно, меняйте другое. До 2026-09-25 здесь было
> `1.22.10` (IMP-02). На сервере проверьте `GO_VERSION` в `deploy/deploy.env`:
> он в `.gitignore` и мог остаться со старым значением.
>
> **Окончания строк.** `deploy.sh` хранится в git с CRLF (IMP-29) — как сервер
> его запускает, из репозитория не видно. **[НЕИЗВЕСТНО]**

### nginx: ключевые места

| Что | Значение |
|-----|----------|
| upstream `go_api` | `127.0.0.1:8081`, `keepalive 16` |
| Корень SPA | `/var/www/corporate.admsr.ru/dist` |
| Go allowlist | `location = /api/<файл>.php` (79 блоков) |
| PHP fallback | `location ~ ^/api/(.+\.php)$` → `unix:/run/php/php8.3-fpm.sock` |
| Загрузки | `location /img/` → alias `/var/www/corporate.admsr.ru/public/img/`, `expires 30d` |
| Ассеты Vite | `location /assets/` → `expires 1y`, `Cache-Control: public, immutable` |
| SPA fallback | `location / { try_files $uri $uri/ /index.html; }` |
| TLS | TLSv1.2/1.3, HSTS `max-age=31536000`, сертификаты `/etc/nginx/ssl/admsr.ru/` |
| Лимит тела | `client_max_body_size 25m` |

Точные `location =` имеют приоритет над регуляркой PHP — на этом и держится
переключение на Go. **[ПОДТВЕРЖДЕНО]**

### systemd

`deploy/corporate-go-api.service`: `WorkingDirectory=/var/www/corporate.admsr.ru/backend`,
`ExecStart=…/backend/bin/api`, `Restart=on-failure`, `NoNewPrivileges=true`,
`LimitNOFILE=65535`. `.env` читается самим бинарником из рабочего каталога;
`EnvironmentFile` намеренно закомментирован — комментарий в юните объясняет это
тем, что не нужно наследовать случайный `HTTP_ADDR` из окружения systemd.
**[ПОДТВЕРЖДЕНО — причина в комментарии юнита]**

```bash
systemctl status corporate-go-api
journalctl -u corporate-go-api -n 50 --no-pager
```

### Артефакты альтернативных площадок

В репозитории лежат конфиги хостингов, которые в текущей схеме не участвуют:
`vercel.json`, `public/_redirects` (Netlify), `public/web.config` (IIS),
`public/.htaccess` (Apache). Прод — nginx. Историю их появления код не объясняет.
**[ПОДТВЕРЖДЕНО, причина НЕИЗВЕСТНА]**

## Порядок проверки после изменений

### Фронтенд
1. `npm run build` — должен пройти без ошибок.
2. `npm run dev` и ручная проверка затронутых экранов.
3. Для UI: обе цветовые схемы, минимум два primary-цвета, шрифты Inter и
   Unbounded, ширины ~375 / 768 / 1280 / 1600.

### Backend
1. `cd backend && go build ./... && go vet ./... && go test ./...`.
2. Если добавлен эндпоинт — проверить nginx allowlist и dev-прокси Vite.
3. Проверить, что аноним получает 401 на закрытых путях.

### БД
1. Миграция идемпотентна (её прогонят заново на каждом деплое).
2. `npm run test:courses`, если затронут модуль курсов.

### Всегда
Обновить затронутые документы в `docs/` в рамках той же задачи.
