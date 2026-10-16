# API

## Формат

Все эндпоинты — JSON over HTTP по путям вида `/api/<имя>.php`. Расширение `.php`
сохранено как URL-контракт: за ним может стоять как PHP-скрипт, так и Go-хендлер.
**[ПОДТВЕРЖДЕНО]**

Конверт ответа Go (`backend/internal/httpx/json.go`):

```json
{ "success": true, "message": "OK", "data": { … } }
```

При ошибке — `success: false`, `message` на русском, `data: null`, HTTP-код
соответствует ситуации.

Стандартные коды из `backend/internal/handlers/section.go`:

| Код | Сообщение | Когда |
|-----|-----------|-------|
| 401 | `Требуется авторизация` | нет/невалидна сессия |
| 403 | `Недостаточно прав` / `Недостаточно прав для этого раздела` | сессия есть, прав нет |
| 405 | `Метод не поддерживается` | неверный HTTP-метод |
| 500 | `Ошибка подключения к БД` | сбой БД |

> Часть PHP-скриптов отвечает полями `error` вместо `message` — клиент
> `src/tests/api.ts:27` учитывает оба варианта. **[ПОДТВЕРЖДЕНО]**

## Маршрутизация Go ↔ PHP

Три списка путей должны совпадать. На 2026-09-22 они **совпадают полностью:
79 эндпоинтов** в каждом. **[ПОДТВЕРЖДЕНО — сверено скриптом]**

| Список | Файл |
|--------|------|
| Go-маршруты | `backend/cmd/api/main.go` |
| Прод-allowlist nginx | `deploy/nginx-corporate.admsr.ru.conf` + `deploy/nginx-go-api-wave2.conf` |
| Dev-прокси Vite | `vite.config.js` |

**При добавлении Go-эндпоинта обновляйте все три.** Иначе запрос молча уйдёт в
PHP-реализацию (или в 404, если PHP-файла нет).

## Каталог эндпоинтов

### Обслуживаются Go (82)

| Группа | Эндпоинты | Хендлер |
|--------|-----------|---------|
| Здоровье | `health.php` | `health.go` |
| Аутентификация | `auth.php`, `logout.php`, `check-auth.php`, `heartbeat.php`, `session_bootstrap.php` | `auth.go` |
| Контент | `news.php`, `events.php`, `gallery.php`, `gallery_base.php`, `Upload/upload.php` | `news.go`, `events.go`, `gallery.go` |
| Календарь | `event_rsvp.php`, `calendar_entries.php` | `calendar_personal.go` |
| Комментарии к новостям | `news_comments.php` | `news_comments.go` |
| Люди | `users.php`, `profile.php`, `profile_wall.php`, `profile_extras.php`, `profile_avatar.php`, `feedback.php`, `birthdays.php`, `notifications.php` | `users.go`, `profile.go`, `profile_wall.go` + `birthday_greetings.go`, `profile_extras.go`, `profile_avatar.go`, `feedback.go`, `birthdays.go`, `notifications.go` |
| ОФО | `ofo.php`, `ofo_seats.php`, `ofo_tree.php`, `ofo_positions.php` | `ofo.go` |
| Отсутствия | `absence_journal.php` | `absence.go` |
| Портал | `portal_groups.php`, `portal_my_permissions.php`, `portal_services.php` | `portal.go`, `portal_services.go` |
| Синхронизация | `sync.php` | `sync.go` |
| Тесты (легаси-модуль) | `tests_list/save/publish/unpublish/delete/direct/submit/stats/participant/by_token.php` | `tests_routes.go` |
| Попытки тестов | `tests_attempt_start/save/get/finish.php` | `tests_routes.go`, `attempt_review.go` |
| Формы (второй модуль) | `forms.php`, `forms_list/publish/submit/report/archive/delete.php` | `forms.go` |
| LMS | `courses_list/get/create/update/delete/publish/unpublish/archive/duplicate/readiness/for_me.php`, `course_history.php`, `course_assignment_cancel.php`, `course_assignments_list.php`, `course_materials_order.php`, `course_topics_*`, `course_materials_*`, `course_tests_*`, `course_assign*`, `course_admin_*`, `course_enrollment_*`, `course_start/topic_get/material_open/material_heartbeat/material_complete/next_action/result.php`, `course_file.php` (GET, файлы материалов) | `courses.go` |

### Только PHP (Go-реализации нет)

Нет. Последние семь — `course_history`, `course_assignment_cancel`, `course_assignments_list`,
`course_materials_order`, `courses_archive`, `courses_duplicate`, `courses_readiness` — перенесены в Go
2026-10-02 (`courses_extra.go`, `courses/duplicate.go`, ADR-047); SPA их не вызывает. `auth_context.php`,
`courses_common.php`, `tests_common.php` — общие хелперы PHP, не эндпоинты. Что осталось от PHP и как его
снять — [php-decommission.md](php-decommission.md). `course_file.php` перенесён в Go 2026-09-28 —
страница материала показывает файлы через него.

### Только Go (PHP-файла нет)

`portal_services.php`. Раздел «Сервисы» без запущенного Go API отработает на
`FALLBACK_PORTAL_SERVICES`. **[ПОДТВЕРЖДЕНО]**

## Авторизация по эндпоинтам

Сведено из вызовов `requireUser` / `requireAdmin` / `requireSection` /
`CanEditSection` в `backend/internal/handlers/`. **[ПОДТВЕРЖДЕНО]**

| Эндпоинт | GET | POST / PUT / DELETE |
|----------|-----|---------------------|
| `health.php` | публично | — |
| `auth.php`, `logout.php`, `check-auth.php`, `heartbeat.php` | — | публично (сам механизм входа) |
| `session_bootstrap.php` | — | только для текущей сессии; чужой `id` в теле → 403 |
| `news.php` | публично (`?search=`, `?category=` — подстрока, `?excludeCategory=` — без категории целиком, им вкладка «Новости» скрывает девблоги); `?action=reactors` — `requireUser` | секция `news`; `?action=react`, `?action=like` — `requireUser` (личность из сессии); `?action=view` — публично |
| `events.php` | публично; в ответе `location` (пусто — не указано), `slots` — времена мероприятия `[{id, time_start, time_end, max_participants, participants_count}]` в порядке показа (V23, ADR-056), `participants_count` — всего | секция `events`; тело дополнительно `location` (до 300) и `slots: [{id?, time_start, time_end, max_participants}]` — до 20, «ЧЧ:ММ», лимит ≥ 1 (пусто/0 — без ограничения). При правке `slots` — полный список: с `id` — обновить, без — добавить, отсутствующие удаляются вместе с записями на них; ключа нет — времена не меняются. Без V22/V23 — 503 (ADR-054, ADR-056) |
| `gallery.php`, `gallery_base.php` | публично | секция `gallery` |
| `Upload/upload.php` | — | `requireUser` (любой вошедший; секцию не проверяет — IMP-61) |
| `birthdays.php` | публично (guard'а в `handleGet` нет; данные нужны и киоску). Элемент: `{fio, month, day, avatar, userId}`; `userId` — id **активной** учётной записи с тем же ФИО, `null` — не нашлась или нашлась дважды (поздравить некого) | секция `birthdays` |
| `absence_journal.php` | `requireUser` | `requireUser` + секция `absence_journal` **или** своя запись (`DELETE` — своя активная) |
| `users.php` | `requireAdmin` | `requireAdmin` |
| `profile.php` | `requireUser`; `?view=page` добавляет `ofoName`, `birthday` (из xlsx дней рождения по ФИО), `about`, `interests`, `colleagues`, `courses` (завершённые; `enrollmentId` — только владельцу), `absences` (последние 5), `wishes`, `awards` | владелец записи либо админ; пишет только `avatar_url`, `about`, `interests` (в `profile_about`) и — **один раз** — `ofo` и `role` (пока ОФО не задано / должность пуста). ФИО, телефон и почта из тела **игнорируются** (ADR-041); неприсланные поля не трогаются |
| `profile_avatar.php` | — | `requireUser`, только себе (личность из сессии): multipart, поле `avatar`, JPEG/PNG/WebP до 5 МБ → квадрат 512×512 JPEG в `/img/FullPic/avatars/uploads/<id>-<hex>.jpg`, сразу становится аватаром; прежний загруженный файл удаляется |
| `event_rsvp.php` | `?eventId=` — кто записался `[{id, name, avatar_url, joinedAt, slotId}]`, только секция `events` (ADR-054) | `requireUser`; GET — свои записи `{eventIds, items: [{eventId, slotId}]}`; POST `{eventId, slotId?, joined}` — запись на время (нет `slotId` — первое), перезапись на другое, отмена; одно время на мероприятие; лимит времени — под блокировкой его строки, мест нет — 409; ответ — `{eventId, joined, slotId, participantsCount, slots: [{id, participantsCount}]}` (ADR-056) |
| `calendar_entries.php` | — | `requireUser`; GET — свои записи (с `color`); POST `create`, `update` `{id, …}` (V19), `delete` — только свои (чужую изменить или удалить нельзя, ответ 404). `color` — `#rrggbb` или пусто (цвет по типу) |
| `profile_extras.php` | — | `requireUser`; `wish_add` — только себе, `wish_delete` — автор или админ, `award_add` / `award_delete` — только админ (`user_group = admin`) |
| `profile_wall.php` | `requireUser` (`?userId=` — стена, `?action=reactors`, `?action=my_greetings` — мои поздравления за этот и прошлый год `[{postId, userId, year}]`) | `requireUser`; `action`: `create` — любой на любую стену, `greet` `{userId, content}` — поздравление с днём рождения (ADR-049): не себе, именинник активен и есть в xlsx, сегодня — день рождения или до 3 дней после, одно от автора в год (иначе 409), `update` — только автор, `delete` — автор, владелец стены или админ, `react` — любой. `create` и `greet` создают уведомление владельцу стены. Мат в `create`/`update`/`greet` — 422 с задачей `data.profanity` (ADR-053) |
| `news_comments.php` | `requireUser` (в киоске комментариев нет): `?newsId=&sort=popular\|new&offset=` — `{total, rootsTotal, items}` по 20; `?action=replies&rootId=` — вся ветка по времени; `?action=thread&id=` — ветка комментария `{root, replies}` (переход из уведомления); `?action=reactors&id=&reaction=` | `requireUser`; `action`: `create` `{newsId, content}` или `{replyToId, content}` — ответ встаёт в ветку того комментария (два уровня), получателю — уведомление `comment_reply`; `update` — только автор; `delete` — автор или секция `news` (вкл. админа): с ответами — мягко («Комментарий удалён»), иначе физически; `react` — любой. Текст до 2000 символов, без HTML. Без V18 — 503 с текстом (ADR-050). Мат в `create`/`update` — 422 с задачей `data.profanity`, повтор с `challengeToken` + `challengeAnswer` (ADR-053) |
| `devblog.php` | `requireUser` — последний девблог, который вы не закрыли (`{newsId, title, html, imagePath, date, publishedAt, version, reactions}` или `null`); `?action=draft` — `requireAdmin`, общий черновик `{title, body, imagePath, releaseVersion, suggestedVersion, version, updatedAt, updatedBy}` (`version` — номер правки черновика, `releaseVersion` — версия выпуска, `suggestedVersion` — последняя опубликованная + 0.0.1, первая — 1.0.0) | `requireAdmin`: `save` `{title, body, imagePath, releaseVersion, version, force?}` — устаревшая версия → 409 с актуальным черновиком; `publish` `{…, releaseVersion, html, version, force?}` (версия обязательна, «X.Y.Z») — новость «Девблог» + уведомления всем активным + очистка черновика (транзакция), 409 при устаревшей версии. `requireUser`: `dismiss` `{newsId}` — закрыть окошко и прочитать уведомление. Обложка — только путь `/img/…`, иначе стандартная (ADR-052) |
| `notifications.php` | `requireUser`; `?limit=` (20, до 50) → `{unread, items: [{id, kind, postId, commentId, newsId, createdAt, read, actor{id,name,avatar_url} | null, excerpt, reminder}]}` — только свои. Перед ответом создаёт напоминания `event_reminder` о завтрашних личных событиях и мероприятиях, куда записан, и убирает устаревшие (V19, ADR-051); у напоминания `actor: null`, `reminder: {title, date, timeStart, location, color, calendarEntryId, eventId}` | `requireUser`; `action`: `read` `{ids}` — только свои (чужие id молча не трогаются), `read_all`. Ответ `{unread}`. Без V17 — 503 с текстом |
| `feedback.php` | — | `requireUser` |
| `ofo*.php` | `requireUser` | `requireUser` |
| `portal_groups.php` | `requireUser` / `requireAdmin` | `requireAdmin` |
| `portal_my_permissions.php` | `requireUser` | — |
| `portal_services.php` | `requireUser` | `requireAdmin` |
| `sync.php` | — | общий секрет в заголовке `X-Sync-Secret` |
| `tests_*` | личность только из сессии; публичные потоки (список опубликованных, отправка по токену) доступны гостю с `viewer=0` | операции владельца требуют сессии |
| `forms*.php` | `formsRequireUser` / `formsRequireEditor` (право секции `tests`) | то же |
| `courses_*`, `course_*` | `requireUser` + проверка enrollment; админ-операции — секция `courses` + категория курса | то же |

Детали и история закрытия дыр — [auth-and-permissions.md](auth-and-permissions.md)
и [PROJECT_AUDIT.md](PROJECT_AUDIT.md).

## CORS

`backend/internal/httpx/cors.go` разрешает с `Allow-Credentials: true` только
`http://localhost:5173`, `:5174` и их `127.0.0.1`-варианты (для Vite).
Для прочих origin'ов ставится `Access-Control-Allow-Origin: http://localhost:5173`
без `Allow-Credentials`. Разрешённые заголовки: `Content-Type`, `Authorization`,
`X-Session-Token`. **[ПОДТВЕРЖДЕНО]**

> Заголовок `X-User-Id`, который шлёт `src/tests/api.ts:22`, в
> `Access-Control-Allow-Headers` **не входит**. В проде это не мешает
> (same-origin), в dev — запрос идёт через прокси Vite, тоже same-origin.
> На реальный кросс-доменный сценарий не рассчитано. **[ПОДТВЕРЖДЕНО]**

## Курсорная пагинация

`backend/internal/handlers/cursor.go` + `src/composables/useCursorFeed.ts`.

Ответ страницы:

```json
{ "items": [...], "nextCursor": "…" | null, "hasMore": true | false }
```

Клиент дедуплицирует по `getId`, отменяет предыдущий запрос через
`AbortController`, хранит курсор в сторе (для новостей — Pinia `newsFeed`).

## Проверка живости

```bash
curl -fsS -H "Host: corporate.admsr.ru" http://127.0.0.1/api/health.php
```

Ожидается `{"ok":true,"service":"corporate-portal","database":"ok","backend":"go",…}`.
Поле `backend: "go"` — признак того, что запрос дошёл до Go, а не до PHP.
**[ПОДТВЕРЖДЕНО — `backend/README.md`, `deploy/deploy.sh`]**

## Добавление нового эндпоинта: чек-лист

1. Хендлер в `backend/internal/handlers/<домен>.go`.
2. **Guard в самом начале метода** (`requireUser` / `requireAdmin` / `requireSection`)
   — middleware-цепочки нет, забыть легко.
3. Регистрация в `backend/cmd/api/main.go`.
4. `location = /api/<имя>.php { proxy_pass http://go_api; … }` в
   `deploy/nginx-go-api-wave2.conf` (копировать блок целиком, включая
   `proxy_set_header Authorization $http_authorization`).
5. Строка в `server.proxy` в `vite.config.js` → `http://127.0.0.1:8080`.
6. Клиентская обёртка: `apiSessionFetch` для авторизованного вызова.
7. `go build ./... && go vet ./... && go test ./...`.
8. Обновить этот документ.
