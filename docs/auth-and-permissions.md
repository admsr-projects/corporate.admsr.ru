# Аутентификация и права доступа

## Вход

1. `POST /api/auth.php` с `{ login, password }` — `src/pages/login.vue:41-47`.
2. Если записи в `user_info` нет, Go обращается во внешнюю систему **ASU**
   (`ASU_URL`, заголовок `X-Sync-Secret: ASU_SECRET`) и создаёт пользователя —
   `backend/internal/handlers/auth.go:279`.
3. Успех → создаётся серверная сессия: случайные 32 байта → hex-токен, в
   `user_sessions` кладётся `sha256(token)`; клиенту уходят одновременно
   - httpOnly-cookie `corp_session` (`SameSite=Lax`, `Secure` при TLS или
     `X-Forwarded-Proto: https`), и
   - поле `sessionToken` в теле ответа.

   `backend/internal/auth/session.go:75-127`. **[ПОДТВЕРЖДЕНО]**
4. SPA сохраняет профиль в `localStorage['auth-user']`, токен — в
   `localStorage['auth-session']` (`login.vue:50-54`, `useAuthSession.ts:25-32`).
5. Редирект: `/welcome`, если профиль неполный (нет ОФО / должности / аватара —
   `useOnboarding.ts:26-31`), иначе `/`.

## Два параллельных способа предъявить себя

| Способ | Кто использует | Извлечение на сервере |
|--------|----------------|-----------------------|
| Cookie `corp_session` | обычные `fetch('/api/…')` — новости, профиль, пользователи, `useTestsStore` | `auth.ExtractToken` → `r.Cookie("corp_session")` |
| `Authorization: Bearer <token>` + `X-Session-Token` | `apiSessionFetch` — курсы, сервисы, группы, обратная связь, мутации журнала | `auth.ExtractToken` → заголовки |

`ExtractToken` проверяет источники в порядке: `Authorization: Bearer` →
`X-Session-Token` → cookie — `backend/internal/auth/session.go:60-73`.
**[ПОДТВЕРЖДЕНО]**

## Проверка сессии на сервере

`Service.CurrentUser` (`session.go:160-201`) одним запросом джойнит
`user_sessions` и `user_info` и отклоняет запрос, если:

- сессии нет, либо `revoked_at IS NOT NULL`;
- `user_info.status = false` (заблокирован);
- `user_info.auth = false`;
- `expires_at <= now()`.

В последних трёх случаях сессия сразу помечается `revoked_at = now()`.
При успехе обновляются `user_sessions.last_seen_at` и `user_info.last_activity`.

## Три уровня прав

### 1. Суперадминистратор

`user_info.user_group = 'admin'` — `auth.IsAdmin` (`session.go:203-205`).
Даёт все секции и все категории курсов (`permissions.go:UserSections`,
`UserCourseCategories`).

### 2. Права по разделам через группы

```sql
portal_group_members(user_id) → portal_group_permissions(section_key)
```

Белый список секций — в двух местах, синхронизируются вручную:

| Сторона | Файл |
|---------|------|
| Backend | `backend/internal/auth/permissions.go:10-12` |
| Frontend | `src/pages/Admin/portalSections.ts:2-10` |

Ключи (на 2026-09-22 совпадают): `news`, `events`, `gallery`, `courses`,
`tests`, `absence_journal`, `birthdays`. **[ПОДТВЕРЖДЕНО]**

### 3. Категории курсов

Дополнительное сужение внутри секции `courses`:
`portal_group_course_categories.category_key`. Список из двух значений
(`Кадровая деятельность`, `Безопасность`) продублирован в
`backend/internal/auth/permissions.go:14-16` и
`src/pages/Courses/courseCategories.ts:2-5`. **[ПОДТВЕРЖДЕНО]**

`CanEditCourseCategory` отказывает, если категория пустая или `NULL`
(`permissions.go:106-123`).

## Клиентская сторона

### UI-тоггл роли

`src/stores/role.js` хранит `ui-role` ∈ {`user`, `admin`} в localStorage.
Это **переключатель отображения**, а не право: суперадмин может «побыть
пользователем», чтобы увидеть портал глазами сотрудника.

`useSectionAccess.canEdit` (`src/composables/useSectionAccess.ts:112-120`):

```
если isSuperAdmin  → разрешено только когда currentRole === 'admin'
иначе              → разрешено, если секция есть в списке из портальных групп
```

Обычного пользователя тоггл не касается — он видит права своей группы
независимо от `ui-role`. `AppHeader` сбрасывает роль в `user`, если право на
тоггл пропало (`AppHeader.vue:56-58`). **[ПОДТВЕРЖДЕНО]**

### Откуда берутся права

`useSectionAccess.doLoad`:
1. Сначала — из `localStorage['auth-user']` (поля `isAdmin`, `sections`,
   `courseCategories`) — мгновенный рендер без ожидания сети.
2. Затем — `GET /api/portal_my_permissions.php` через `apiSessionFetch`;
   результат перезаписывает состояние **и** кэш в `auth-user`.
3. При ошибке сети остаётся кэш.

Значения фильтруются по белому списку (`normalizeSections`,
`normalizeCourseCategories`) — подложить произвольный ключ через localStorage
не выйдет. **[ПОДТВЕРЖДЕНО]**

> Даже так права на клиенте — только про видимость. Каждый мутирующий эндпоинт
> перепроверяет их на сервере.

### Гварды роутера

`src/router/index.js:210-294`:

- `meta.public` → пропускаем сразу;
- внутри киоска — редирект на киоск-эквивалент **до** проверки авторизации;
- вне киоска без `auth-user` → `/login`;
- раз в 15 минут (`AUTH_CHECK_INTERVAL`) → `POST /api/check-auth.php`;
- `meta.requiresAdmin` → `isSuperAdmin && currentRole === 'admin'`, иначе `/profile`;
- `meta.requiresSection` → `canEditSection(...)`, иначе `/courses`.

### Фоновая активность

`src/composables/useSessionActivity.ts`:
- heartbeat на `click`/`keydown`/`scroll`/`mousemove`/`touchstart`, навигацию и
  возврат на вкладку, не чаще раза в 4 минуты;
- `check-auth.php` каждую минуту и при `visibilitychange`;
- `auth === false` от сервера → `forceLogout()`.

## UX-001 / ARCH-001 — что было и что стало

**Было.** `check-auth.php` проверяет флаг `user_info.auth` и свежесть
`last_activity`, но **не валидность токена сессии**, а сам токен жил
фиксированные 24 часа от момента создания. Непрерывно активный пользователь
через ~24 часа сохранял «залогиненный» UI, но все требующие сессии запросы
возвращали 401 — и интерфейс сыпал ошибками «Требуется авторизация».

Зафиксировано в [PROJECT_AUDIT.md](PROJECT_AUDIT.md) как известное ограничение.

### Что уже сделано по UX-001 (коммит `bcd9d95`)

Симптом «повторяющаяся ошибка *Требуется авторизация*» устранён единым
обработчиком недействительной сессии. **[ПОДТВЕРЖДЕНО]**

```
apiSessionFetch / apiSessionUpload получили 401
   → попытка выдать новый токен (ensureSessionToken)
   → всё ещё 401
   → triggerUnauthorized()            useAuthSession.ts:190, 210, 219
        → onUnauthorized = forceLogout  зарегистрирован в useSessionActivity.ts:61
             → чистка localStorage
             → sessionStorage['session-expired'] = '1'
             → router.replace({ name: 'login' })
                  → login.vue:28-34 показывает «Сессия истекла. Войдите снова.»
```

Защита от лавины: флаг `unauthorizedFired` пропускает только один вызов, сброс
через 5 секунд (`useAuthSession.ts:81-93`). Аноним (`getAuthUser()?.id` пуст)
обработчик не запускает.

**2. Скользящий TTL сессии.** `CurrentUser` теперь при каждом авторизованном
запросе сдвигает `expires_at` на полный TTL от текущего момента
(`session.go:194-205`). Комментарий в коде: «Активный пользователь не
„отваливается“ посреди работы… а по-настоящему простоявшая дольше TTL сессия
честно истекает по условию выше». **[ПОДТВЕРЖДЕНО]**

### Что по UX-001 осталось

`check-auth.php` (router-guard и авто-логаут) по-прежнему проверяет флаг
`user_info.auth` и свежесть `last_activity`, **а не валидность токена сессии**.
То есть UI может считать пользователя залогиненным, пока сессия уже
недействительна, — но теперь это заканчивается корректным выходом на экран
входа, а не серией ошибок. **[ПОДТВЕРЖДЕНО]**

**Истёкшая сессия и приветствие (исправлено 2026-10-16).** Router-guard на каждом
переходе спрашивает `profile.php?id=` — «заполнен ли профиль» (`checkOnboarding`
в `useOnboarding.ts`). Раньше любая неудача, в том числе 401 при истёкшей сессии,
считалась «профиль не заполнен», и вместо входа открывалось приветствие `/welcome`
(«Здравствуйте, коллега!», место работы, аватар) — сотрудникам казалось, что их
отправили на регистрацию. Теперь 401 → `expireSession()` (`clearAuthStorage` +
`session-expired`) → `/login` с «Сессия истекла. Войдите снова.»; на странице входа с
мёртвыми данными в браузере guard тоже остаётся на входе. Сбой сети/сервера на
приветствие не ведёт. Проверено в браузере: старый код — `/welcome`, новый — `/login`.

### Мелкое расхождение

`forceLogout` (`useSessionActivity.ts:48-56`) удаляет `auth-user` и
`auth-last-check`, но **не** `auth-session`, в отличие от `clearAuthStorage()`.
После авто-логаута в localStorage остаётся мёртвый токен. **[ПОДТВЕРЖДЕНО]**

## Что уже закрыто (не «чинить» повторно)

Из [PROJECT_AUDIT.md](PROJECT_AUDIT.md), все со статусом FIXED+VERIFIED:

| ID | Суть | Как закрыто |
|----|------|-------------|
| SEC-001 | `users/profile/forms/sync/ofo` в Go были без авторизации | добавлены `Auth` и `require*`; `password` убран из выдачи; bcrypt при смене |
| SEC-006 | `session_bootstrap` выдавал сессию по `id` из тела | личность только из текущей сессии |
| SEC-007 | Права редактора форм определялись по заголовку `X-User-Id` | личность из сессии, признак редактора — `CanEditSection(..., "tests")` |
| SEC-008 | Модуль тестов доверял `userId` из тела | `viewer()` только из сессии; `owner_id IS NULL` = доступ запрещён |
| SEC-009 | `absence_journal` был полностью открыт | все методы — `requireUser`; мутации: редактор секции — любые, сотрудник — только свои (удаление — только своей активной). Уточнено 2026-09-28 |

Регрессионные тесты: `backend/internal/handlers/auth_guards_test.go`,
`session_bootstrap_test.go`.

**Открытые проблемы PHP-зоны** (SEC-002…SEC-005) — вне работ по решению
владельца, см. [known-issues.md](known-issues.md).

## Правила при изменении гейтинга

1. Гейтить **и** сервер, и UI. Только UI — не защита.
2. Guard ставить первой строкой метода хендлера: middleware-цепочек нет.
3. Новый `section_key` добавлять **в оба** списка (Go + фронт).
4. Трактовать `owner_id IS NULL` как «доступ запрещён» — этот паттерн уже принят
   в `tests_routes.go` после SEC-008.
5. Не вводить новые способы передачи личности — только сессия.
6. После правок прогнать `go test ./...` в `backend/`.
