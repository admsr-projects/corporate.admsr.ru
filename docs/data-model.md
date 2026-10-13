# Модель данных

СУБД: PostgreSQL 14+, база `corporate_portal`, схема `public`.
Расширение `pgcrypto` (для `gen_random_uuid()`) — `db/migration/V1__init_schema.sql:6`.

## Важно: миграции покрывают не всю схему

`db/migration/` описывает **только** модули форм/тестов, курсов, групп доступа и
портальных сервисов. Базовые таблицы портала созданы вне репозитория и в
миграциях отсутствуют. **[ПОДТВЕРЖДЕНО — сверено по `CREATE TABLE` в миграциях
и по SQL-запросам в `backend/internal/`]**

### Таблицы, существующие в БД, но не создаваемые миграциями

| Таблица | Кто использует |
|---------|----------------|
| `user_info` | вся авторизация, профиль, админка (31 запрос в Go) |
| `news` | `handlers/news.go` |
| `events` | `handlers/events.go` (колонку `album_id` добавляет V7; `time_start`, `time_end`, `location`, `max_participants` — V22) |
| `gallery`, `gallery_base` | `handlers/gallery.go` |
| `ofo_unit`, `ofo_unit_position`, `ofo_position`, `ofo_category`, `ofo_seats`, `ofo` | `handlers/ofo.go` |
| `absence_journal` | `handlers/absence.go` |
| `portal_feedback` | `handlers/feedback.go` |

> Следствие: **развернуть проект «с нуля» одними миграциями нельзя.** Требуется
> дамп/скрипт базовой схемы, которого в репозитории нет. Зафиксировано в
> [known-issues.md](known-issues.md). **[ПОДТВЕРЖДЕНО]**

Колонки `user_info`, которые читает Go (`backend/internal/auth/session.go:170-176`):
`id`, `login`, `firstname`, `surname`, `lastname`, `email`, `phone`, `ofo`,
`user_group`, `role`, `status`, `auth`, плюс `last_activity` (обновляется при
каждом `CurrentUser`) и `password`.

> `user_info.role` хранит **должность** сотрудника, а не роль доступа. Роль
> доступа — `user_group`. Эта путаница была источником SEC-007
> (см. [PROJECT_AUDIT.md](PROJECT_AUDIT.md)). **[ПОДТВЕРЖДЕНО]**

## Миграции

Стиль Flyway, идемпотентные (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`).
Применяются в `deploy/deploy.sh:231-261` циклом `psql -f` по `V*.sql` при заданном
`PGPASSWORD`. Таблицы учёта применённых миграций нет — идемпотентность
обеспечивается самим SQL. **[ПОДТВЕРЖДЕНО]**

| Файл | Содержание |
|------|-----------|
| `V1__init_schema.sql` | Модуль форм №2: `users`, `forms`, `questions`, `question_options`, `form_responses`, `response_answers` (UUID-ключи) |
| `V2__tests_module.sql` | Легаси-модуль тестов: `test_forms`, `test_questions`, `test_options`, `test_audience_ofo`, `test_audience_users`, `test_attempts`, `test_answers`, `test_answer_options` (bigint-ключи) |
| `V3__tests_link.sql` | Публичные ссылки по токену (`/t/:token`) |
| `V4__courses_module.sql` | `user_sessions` + 12 таблиц `course_*` |
| `V5__portal_access_groups.sql` | `portal_access_groups`, `portal_group_permissions`, `portal_group_members` |
| `V6__portal_group_course_categories.sql` | `portal_group_course_categories` |
| `V7__events_gallery_album.sql` | `events.album_id → gallery(id)` + частичный индекс |
| `V8__news_feed_index.sql` | `idx_news_date_id_desc` под курсорную ленту |
| `V9__course_certificate.sql` | `course_versions.generate_certificate boolean` |
| `V10__portal_services.sql` | `portal_services` + сид трёх внутренних сервисов |
| `V11__news_reactions.sql` | `news_reactions (news_id, user_id, reaction)` — реакции сотрудников на новости |
| `V13__test_question_pairing.sql` | `test_questions.type` += `match`, `classify`; `test_options.role` (`option`/`item`/`target`), `target_option_id` |
| `V12__course_assign_all.sql` | `course_assignments`: `target_type = 'all'`, колонка `deadline_days`, индекс действующих назначений |
| `V21__devblog_version.sql` | `release_version text` в `devblog_draft` и `devblog_posts` — версия выпуска девблога «X.Y.Z» |
| `V20__devblog.sql` | `devblog_draft` (одна строка `id = 1`: `title`, `body_md`, `image_path`, `version`, `updated_by`, `updated_at`) — общий черновик; `devblog_posts (news_id, body_md, published_by, published_at)`; `devblog_dismissed (news_id, user_id)` — кто закрыл окошко; `notifications.news_id` (вид `devblog`). `news_id` — soft ref; при удалении новости `news.go` чистит девблог и уведомления |
| `V22__events_schedule_limit.sql` | `events.time_start`, `time_end` (`'HH:MM'`), `location`, `max_participants` (NULL — без ограничения; CHECK > 0); индекс `event_rsvps (event_id, created_at)` для списка записавшихся |
| `V19__calendar_colors_reminders.sql` | `calendar_entries.color` (`#rrggbb` или NULL); `notifications.actor_id` — NULL допустим (системные); `notifications.calendar_entry_id → calendar_entries ON DELETE CASCADE`, `event_id → events ON DELETE CASCADE`, `event_date`; уникальный частичный индекс напоминаний `(user_id, COALESCE(calendar_entry_id,0), COALESCE(event_id,0), event_date) WHERE kind = 'event_reminder'` |
| `V18__news_comments.sql` | `news_comments (news_id, root_id → news_comments, reply_to_id → news_comments, reply_to_user_id, author_id, content, deleted_at)` — комментарии к новостям, два уровня (`root_id IS NULL` — комментарий, иначе ответ в ветке); `news_comment_reactions (comment_id → news_comments ON DELETE CASCADE, user_id, reaction)`; `notifications.comment_id → news_comments ON DELETE CASCADE` (вид `comment_reply`). `news_id` — soft ref; при удалении новости комментарии удаляет `news.go` |
| `V17__birthday_greetings_notifications.sql` | `wall_posts.birthday_year` — запись является поздравлением с днём рождения этого года (год дня рождения), уникальный индекс `(owner_id, author_id, birthday_year)` — одно поздравление от автора в год; `notifications (user_id, actor_id, kind, post_id → wall_posts ON DELETE CASCADE, read_at)` — колокольчик, `kind`: `wall_post` / `birthday_greeting` |
| `V16__calendar_personal.sql` | `event_rsvps (event_id → events, user_id)` — запись на мероприятие; `calendar_entries (user_id, source, date_key, title, time_start, time_end, location)` — личные записи и встречи (до 500 на человека, проверяет Go) |
| `V15__profile_extras.sql` | `profile_wishes (user_id, text)` — желания, до 20 на человека (проверяет Go); `profile_awards (user_id, title, description, awarded_on, issued_by)` — награды, выдаёт админ |
| `V14__profile_wall.sql` | Профиль: `profile_about (user_id, about, interests)`, `wall_posts (owner_id, author_id, content)` — стена, простой текст; `wall_post_reactions (post_id, user_id, reaction)` — ключи реакций те же, что у новостей |

## Ключевые группы таблиц

### Сессии

`public.user_sessions` (`V4`, строки 12-27):

| Колонка | Тип | Примечание |
|---------|-----|------------|
| `id` | bigint identity | PK |
| `user_id` | bigint | soft-ref на `user_info.id`, без FK |
| `token_hash` | text **UNIQUE** | sha256 от выданного токена; сам токен в БД не хранится |
| `created_at`, `last_seen_at` | timestamptz | |
| `expires_at` | timestamptz | `now() + AUTH_SESSION_TTL_HOURS` (24 ч). **Скользящий**: каждый авторизованный запрос сдвигает срок на полный TTL от текущего момента (`session.go:194-205`) |
| `revoked_at` | timestamptz NULL | отзыв при logout и при невалидном пользователе |
| `ip_address`, `user_agent` | text | UA обрезается до 500 рун |

Индексы: `user_sessions_user_idx` и `user_sessions_expires_idx` — частичные,
`WHERE revoked_at IS NULL`.

### Права доступа

```
portal_access_groups (id, name UNIQUE lower(name), description)
  ├─ portal_group_permissions   (group_id, section_key)          PK(group_id, section_key)
  ├─ portal_group_members       (group_id, user_id)              PK(group_id, user_id)
  └─ portal_group_course_categories (group_id, category_key)
```

`section_key` не ограничен FK или CHECK — допустимые значения задаются в коде
(`backend/internal/auth/permissions.go:PortalSections`), лишние строки просто
игнорируются при чтении (`UserSections` фильтрует по белому списку).
**[ПОДТВЕРЖДЕНО]**

### Формы — два независимых набора

| Модуль | Таблицы | Тип ключа | Используется на |
|--------|---------|-----------|-----------------|
| Легаси (основной портал) | `test_forms`, `test_questions`, `test_options`, `test_audience_*`, `test_attempts`, `test_answers`, `test_answer_options` | bigint | `/tests`, `/t/:token` |
| Второй (V1) | `forms`, `questions`, `question_options`, `form_responses`, `response_answers`, `users` | uuid | `/kiosk/tests` |

Таблица `public.users` (V1) — **не** `user_info`: это отдельный реестр модуля
форм с `external_user_id bigint UNIQUE` как мостом к `user_info.id`
(`V1__init_schema.sql:10-22`). **[ПОДТВЕРЖДЕНО]**

### Курсы (V4)

```
course_courses
  └─ course_versions            (версия курса: статус draft|published|archived, правила прохождения)
       ├─ course_topics
       │    ├─ course_materials
       │    └─ course_test_links (type='topic')
       └─ course_test_links      (type='final')

course_assignments ──> course_enrollments
                          ├─ course_topic_progress
                          ├─ course_material_progress
                          ├─ course_learning_sessions
                          ├─ course_test_attempt_links ──> test_attempts
                          └─ course_completions   (снимок результата)
course_audit_logs
```

Подробные определения колонок, индексы и FK — [courses-database.md](courses-database.md).

Ссылки на `user_info` / `ofo_unit` во всём модуле — **soft refs без FK**;
целостность обеспечивается на стороне API (`V2__tests_module.sql:6`,
`V4__courses_module.sql:4`). Причина, указанная в самих миграциях: эти таблицы
синхронизируются извне (`sync.php`). **[ПОДТВЕРЖДЕНО — причина прямо в комментарии]**

### Портальные сервисы (V10)

`portal_services`: `kind` ∈ {`internal`, `external`} с CHECK'ом
`portal_services_internal_chk` — у `internal` обязателен `path`, у `external` —
`external_url`. Индекс `(is_enabled, sort_order, id)`. Сид добавляет три
внутренних сервиса, если их `internal_key` ещё нет.

## Соглашения

- Все таблицы в схеме `public`, явно квалифицируются в запросах Go.
- Временные метки — `timestamptz NOT NULL DEFAULT now()`.
- Новые таблицы — `bigint GENERATED ALWAYS AS IDENTITY` (V4+) или `bigserial` (V10);
  старые модули форм — `uuid DEFAULT gen_random_uuid()`.
- Статусы — `text` + `CHECK (… IN (…))`, а не enum-типы.

## Что проверять после изменения схемы

1. Новая миграция `V<N>__<имя>.sql`, **идемпотентная** (её прогонят повторно).
2. Затронутые Go-хендлеры (`grep` по имени таблицы в `backend/internal/`).
3. Если эндпоинт ещё обслуживается PHP — соответствующий скрипт в `api/`
   (править нельзя по решению владельца: обсудить перенос в Go).
4. Типы на фронте (`src/composables/use*Data.ts`, `useCoursesStore.ts`,
   `src/tests/types.ts`).
5. Индексы под новые фильтры/сортировки.
6. `npm run test:courses` — если затронут модуль курсов.
