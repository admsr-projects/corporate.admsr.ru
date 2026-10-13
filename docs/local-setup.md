# Локальный запуск, конфигурация и команды

## Требования

| Инструмент | Нужен для | Источник требования |
|-----------|-----------|---------------------|
| Node.js 20 LTS+, npm 10+ | Фронтенд | `README.md`; на машине разработчика проверено Node 22.20.0 / npm 10.9.3 |
| Go (версия из `backend/go.mod`) | Go API | `backend/go.mod`: `go 1.26.5` |
| PostgreSQL 14+ | БД | `README.md`, `db/migration/` |
| PHP 8.2+ (на сервере 8.3) | Легаси-API | `deploy/nginx-corporate.admsr.ru.conf:287` |

Для фронтенд-разработки PHP и PostgreSQL локально **не обязательны**: dev-прокси
Vite отправляет непереехавшие пути на тестовый сервер. **[ПОДТВЕРЖДЕНО]**

## Запуск одной командой (Windows)

`scripts\dev.cmd` (или `powershell -ExecutionPolicy Bypass -File scripts\dev.ps1`) — запускает Docker Desktop, если он
не запущен, поднимает локальный Postgres (`db/local-dev`, порт 55432), собирает и запускает Go API (`:8080`) и Vite
(`:5173`). Ctrl+C останавливает Vite и API, база остаётся работать. Ключи: `-NoBuild` (не пересобирать API), `-DryRun`
(проверить предусловия). Скрипт завершится с понятной ошибкой, если порты 8080/5173 заняты. Тот же набор переменных
`DB_*`, что в `.claude/launch.json`.

## Фронтенд

```bash
npm install
npm run dev
```

Откроется `http://localhost:5173` (`vite.config.js:57`).

```bash
npm run build      # production-сборка в dist/
npm run preview    # локальный просмотр сборки
```

`npm run build` перегенерирует `auto-imports.d.ts` и `components.d.ts` — они
отслеживаются git, но перечислены в `.gitignore`, поэтому после сборки
показываются как изменённые. `deploy/deploy.sh:149-163` сбрасывает их перед
`git pull`. **[ПОДТВЕРЖДЕНО]**

### Dev-прокси

`vite.config.js:58-146`:

- ~90 точных путей `/api/<файл>.php` → `http://127.0.0.1:8080` (локальный Go API);
- всё остальное `/api` и `/img` → тестовый сервер во внутренней сети
  (конкретный адрес — в `vite.config.js`, в документацию не выносится) с
  `secure: false` (комментарий в конфиге: игнорируется несовпадение
  wildcard-сертификата с IP, только для dev). **[ПОДТВЕРЖДЕНО]**
- у `/img` есть `bypass`: если файл существует в `public/` (туда пишет локальный Go API —
  `UPLOAD_DIR=public`), он отдаётся локально, а не уходит на тестовый сервер. Без этого
  загруженные локально аватары и картинки в dev не открывались. **[ПОДТВЕРЖДЕНО]**

> **Несогласованность портов.** Dev-прокси ждёт Go на `:8080`,
> `backend/.env.example` предлагает `HTTP_ADDR=127.0.0.1:8081`, прод-nginx
> проксирует на `:8081`. Значение по умолчанию в коде — `:8080`
> (`backend/internal/config/config.go:26`). Для локальной работы с Vite нужен
> именно `:8080`. **[ПОДТВЕРЖДЕНО]**

## Go API

```bash
cd backend
cp .env.example .env     # заполнить DB_*
make run                 # go run ./cmd/api
```

Проверка:

```bash
curl -s http://127.0.0.1:8080/api/health.php
```

Другие цели `backend/Makefile`: `make build` (→ `bin/api`), `make tidy`, `make test`.

`.env` читается самим бинарником: `config.LoadDotEnv(".env")` и
`config.LoadDotEnv("backend/.env")` — `backend/cmd/api/main.go:20-21`, то есть
запускать можно как из `backend/`, так и из корня репозитория. **[ПОДТВЕРЖДЕНО]**

### Переменные окружения Go API

Читаются в `backend/internal/config/config.go`. Значения по умолчанию указаны как
есть; **реальные секреты в документацию не выносятся**.

| Переменная | Назначение | Значение по умолчанию | Безопасный пример |
|-----------|------------|----------------------|-------------------|
| `HTTP_ADDR` | Адрес прослушивания | `:8080` | `127.0.0.1:8080` |
| `DB_HOST` | Хост PostgreSQL | `localhost` | `localhost` |
| `DB_PORT` | Порт PostgreSQL | `5432` | `5432` |
| `DB_NAME` | Имя базы | `corporate_portal` | `corporate_portal` |
| `DB_USER` | Пользователь БД | `myuser` | `portal_app` |
| `DB_PASS` | Пароль БД | пусто | `CHANGE_ME` (только в `.env`, файл в `.gitignore`) |
| `AUTH_SESSION_TTL_HOURS` | TTL сессии в часах | `24` | `24` |
| `ASU_URL` | Эндпоинт внешней системы ASU для поиска сотрудника при первом входе | адрес во внутренней сети | `https://<asu-host>/asu_lookup.php` |
| `ASU_HOST` | Значение заголовка `Host` при запросе к ASU | — | `asu.example.local` |
| `ASU_SECRET` | Общий секрет: отправляется в ASU **и** требуется во входящем `X-Sync-Secret` на `/api/sync.php` | значение по умолчанию задано в коде | `CHANGE_ME` |
| `UPLOAD_DIR` | Корень загрузок (`img/FullPic`, `img/SmallPic`, `birthdays_xlsx/`) | `/var/lib/corporate-app/uploads` | тот же |
| `BIRTHDAYS_DIR` | Явный каталог с `*.xlsx` дней рождения (опционально) | не задана | `${UPLOAD_DIR}/birthdays_xlsx` |

> `ASU_SECRET` выполняет **две** роли: исходящую (заголовок в ASU,
> `handlers/auth.go:279`) и входящую (проверка `X-Sync-Secret` в `sync.go:31`,
> сравнение `subtle.ConstantTimeCompare`). Смена значения затрагивает оба
> направления. **[ПОДТВЕРЖДЕНО]**

> **Важно:** в коде заданы непустые значения по умолчанию для `DB_USER`,
> `ASU_URL` и `ASU_SECRET`. Это значит, что при незаполненном `.env` сервис
> стартует с встроенными значениями, а не падает. Отдельно: `sync.php` при
> пустом `ASU_SECRET` возвращает 401 всем (`sync.go:32`). **[ПОДТВЕРЖДЕНО]**

## Конфигурация PHP-слоя

`api/config.local.php` создаётся из `api/config.local.php.example`; файл в
`.gitignore`. Его использует `api/health.php`. **[ПОДТВЕРЖДЕНО]**

> Остальные скрипты `api/` содержат собственные константы подключения. Это
> зафиксировано как SEC-002 в [PROJECT_AUDIT.md](PROJECT_AUDIT.md) (пароль БД
> в 27 отслеживаемых файлах). PHP-слой по решению владельца не правится.

## npm-скрипты

Из `package.json`:

| Команда | Что делает | Статус |
|---------|-----------|--------|
| `npm run dev` | Vite dev-сервер на `:5173` | ✅ |
| `npm run build` | Production-сборка в `dist/` | ✅ |
| `npm run preview` | Просмотр собранного | ✅ |
| `npm run typecheck` | `scripts/typecheck.mjs`: vue-tsc, сравнение с `scripts/typecheck-baseline.json` | ✅ |
| `npm run images:webp` | `node scripts/generate-events-webp.mjs` | ✅ файл существует |
| `npm run formdata:excel` | `node scripts/formdata-to-excel.mjs` | ✅ файл существует |
| `npm run test:courses` | `php scripts/test_courses.php` — smoke-проверка схемы курсов | ✅ файл существует |
| `npm test` | `echo "Error: no test specified" && exit 1` | заглушка |

Прочие утилиты в `scripts/` (не подключены к npm): `export-gallery-sql.mjs`,
`export-gallery-base-sql.mjs`, `seed_demo_course.php`, `convert_absence_journal.py`,
`replace_absence_user_id_with_internal_id.py`, `_convert_now.py`.

## Отсутствующая оснастка

Подтверждено отсутствие в репозитории. **[ПОДТВЕРЖДЕНО]**

| Чего нет | Следствие |
|----------|-----------|
| Линтера (ESLint/oxlint/Biome) | Стиль кода не проверяется автоматически |
| Форматтера (Prettier) | Форматирование не унифицировано |
| Тестов фронтенда (Vitest/Playwright) | `npm test` — заглушка |
| CI (`.github/workflows`, `.gitlab-ci.yml`) | Проверки запускаются только вручную |

Единственный автоматический набор проверок — Go-тесты (`backend/internal/`,
41 тест-функция в `internal/handlers`, `internal/courses`, `internal/tests`, `internal/media` и `internal/profanity`). См. [quality-and-deploy.md](quality-and-deploy.md).

## Локальная БД в Docker

Когда тестовая БД недоступна (или нужна чистая база), поднимается локальная
PostgreSQL — `db/local-dev/` (добавлено 2026-09-28, ADR-030):

```bash
docker compose -f db/local-dev/docker-compose.yml up -d     # 127.0.0.1:55432
docker compose -f db/local-dev/docker-compose.yml down -v   # сбросить всё
```

При первом старте контейнер применяет по порядку:
`00_legacy_schema.sql` (базовые таблицы, которых нет в `db/migration/` —
IMP-14; восстановлены по SQL-запросам Go, **это не копия продовой схемы**) →
`10_migrations.sh` (все `db/migration/V*.sql`) → `20_seed.sql` (три ОФО, три
пользователя, одна новость). Логины и пароли тестовых пользователей — в
комментарии `20_seed.sql`; вход идёт по `user_info` без обращения к ASU.

Go API подключается к ней переменными окружения, которые перекрывают
`backend/.env` (`LoadDotEnv` не затирает уже заданные):
`DB_HOST=127.0.0.1 DB_PORT=55432 DB_USER=devuser DB_PASS=devpass`.

Чего нет в локальной БД: картинок (`/img/*` dev-прокси отправляет на прод-сервер,
локально это 500 в консоли — на работу портала не влияет), данных ASU.

## Preview-конфигурация для инструментов

`.claude/launch.json` описывает dev-сервер `dev` (`npm run dev`, порт 5173) и
`api` (собранный `backend/bin/api.exe`, порт 8080, с переменными `DB_*` на
локальную БД из раздела выше; перед запуском — `go build -o bin/api.exe ./cmd/api`
в `backend/`). **[ПОДТВЕРЖДЕНО]**

## Ключи localStorage / sessionStorage

Полный перечень клиентского состояния. **[ПОДТВЕРЖДЕНО]**

| Ключ | Хранилище | Где | Что |
|------|-----------|-----|-----|
| `auth-user` | localStorage | `useAuthSession.ts` | Профиль пользователя после входа (+ `sections`, `courseCategories`, `isAdmin`) |
| `auth-session` | localStorage | `useAuthSession.ts` | `sessionToken` для `Authorization: Bearer` |
| `auth-last-check` | localStorage | `router/index.js` | Метка последней проверки `check-auth.php` (интервал 15 мин) |
| `post-login-redirect` | localStorage | `pages/login.vue` | Куда вернуть после входа |
| `ui-role` | localStorage | `stores/role.js` | UI-тоггл роли: `user` \| `admin` |
| `ui-color-mode` | localStorage | `useColorMode.ts` | `light` \| `dark` \| `system` |
| `ui-kiosk-color-mode` | localStorage | `useColorModeSchedule.ts` | `auto` \| `light` \| `dark` (только киоск) |
| `ui-theme-selection:v1` | localStorage | `useUiTheme.ts` | Primary / neutral / шрифт / радиус |
| `ui-app-config:v1` | localStorage | `useAppConfig.ts` | Зеркало темы в форме app-config |
| `ui-zoom:v1` | localStorage | `useUiZoom.ts` | Масштаб интерфейса |
| `portal-dashboard` | localStorage | `App.vue:9` (`UDashboardGroup storage-key`) | Размер/состояние сайдбара |
| `onboarding-complete:v1:<userId>` | sessionStorage | `useOnboarding.ts` | Онбординг пройден в этой сессии вкладки |
| `session-expired` | sessionStorage | `useSessionActivity.ts` → `login.vue` | Показать «Сессия истекла» (незакоммиченная правка на 2026-09-22) |

> Роль (`ui-role`) хранится на клиенте и влияет **только на видимость UI**.
> Серверные права проверяются независимо — см. [auth-and-permissions.md](auth-and-permissions.md).
