# CLAUDE.md — правила работы над corporate.admsr.ru

Корпоративный портал Администрации: Vue 3 + Vite + Nuxt UI 4 SPA, Go API
(`backend/`), легаси PHP API (`api/`), PostgreSQL.

**Полная документация — [`docs/README.md`](docs/README.md).** Там карта: что
читать перед какой задачей. Этот файл — только правила и команды.

---

## Обязательный порядок

### Перед изменениями

1. Прочитать этот файл.
2. Открыть [`docs/README.md`](docs/README.md) и документы **по своей задаче**
   (не всю документацию — там есть маршруты чтения).
3. **Проверить сам код.** Документация могла устареть: код — источник истины.
4. Найти существующую реализацию, которую можно переиспользовать
   ([`docs/modules.md`](docs/modules.md),
   [`docs/design/components.md`](docs/design/components.md)).
5. Для UI-задач дополнительно:
   [`docs/design/tokens.md`](docs/design/tokens.md),
   [`docs/design/layout-and-navigation.md`](docs/design/layout-and-navigation.md),
   [`docs/design/components.md`](docs/design/components.md),
   [`docs/design/states-and-feedback.md`](docs/design/states-and-feedback.md);
   API компонента Nuxt UI сверять по установленной версии
   ([`docs/design/nuxt-ui.md`](docs/design/nuxt-ui.md)).
6. Если есть существенное противоречие или не хватает требований — **сказать об
   этом явно**. Задать вопрос, если от ответа зависит корректность реализации;
   иначе — зафиксировать допущение и продолжить.

### Во время работы

- Соблюдать подтверждённую архитектуру, соглашения и дизайн.
- **Не добавлять незаказанное.** Делать то, о чём попросили.
- Не вводить новые зависимости, архитектурные подходы и визуальные паттерны без
  обоснования и согласования.
- Не выдавать предположения за факты. Не уверен — так и написать.
- Не править документацию ради оправдания произвольного решения.
- Найденные попутно дефекты — **в [`docs/known-issues.md`](docs/known-issues.md)**,
  а не чинить на ходу.

### После изменений

1. Выполнить подходящие проверки (см. «Команды»).
2. Обновить затронутую документацию **в рамках той же задачи**.
3. Существенное новое решение записать в
   [`docs/decisions.md`](docs/decisions.md) — с **действительной** причиной.
   Причина неизвестна — так и пометить.
4. Сообщить: что изменено, что проверено (и как — запуском или чтением кода),
   какие ограничения остались.

---

## Команды

```bash
npm install          # установка зависимостей
npm run dev          # Vite dev-сервер на :5173
npm run build        # production-сборка в dist/
npm run typecheck    # vue-tsc с базовой линией: ошибок не больше, чем в scripts/typecheck-baseline.json
npm run preview      # просмотр сборки
npm run test:courses # smoke-проверка схемы курсов (нужен PHP + БД)
```

```bash
cd backend
make run             # Go API (go run ./cmd/api)
go build ./...       # компиляция
go vet ./...         # статический анализ
go test ./...        # тесты (41 тест-функция)
```

Проверка живости API:

```bash
curl -fsS -H "Host: corporate.admsr.ru" http://127.0.0.1/api/health.php
```

**Чего нет:** линтера, форматтера, тестов фронтенда, CI. `npm test` — заглушка.

**Минимум перед сдачей:** фронтенд — `npm run build && npm run typecheck`; backend —
`go build ./... && go vet ./... && go test ./...`.

---

## Границы

| Зона | Правило | Источник |
|------|---------|----------|
| `api/` (PHP) | **Не изменять.** Все правки backend — только в Go. Эндпоинтов только в PHP не осталось; снятие — [`docs/php-decommission.md`](docs/php-decommission.md) | PROJECT_AUDIT §8, решение владельца 2026-09-21 |
| `src/pages/Kiosk/`, `src/AppKiosk.vue`, `src/components/Kiosk*` | **Не трогать** | там же |
| `components.d.ts`, `auto-imports.d.ts` | Генерируются сборкой, вручную не править | `vite.config.js` |
| `dist/` | Артефакт сборки | — |
| `.cursor/rules/*.mdc` | Личности агентов Cursor, к этому проекту не относятся | — |

---

## Ключевые особенности, о которых нужно помнить

1. **Два backend'а.** URL `/api/*.php` может обслуживаться Go или PHP —
   решает allowlist nginx. Для многих эндпоинтов есть две реализации.
   → [`docs/api.md`](docs/api.md)

2. **Добавляя Go-эндпоинт, обновите три места:** `backend/cmd/api/main.go`,
   `deploy/nginx-go-api-wave2.conf`, `vite.config.js`. Иначе запрос молча уйдёт
   в PHP.

3. **Guard пишется первой строкой хендлера.** Middleware-цепочек нет —
   `requireUser` / `requireAdmin` / `requireSection` вызываются вручную.
   Именно из-за пропущенных guard'ов возникли SEC-001/009.

4. **Личность — только из серверной сессии.** Никаких `userId` из тела и
   `X-User-Id`. → [`docs/auth-and-permissions.md`](docs/auth-and-permissions.md)

5. **Два модуля форм.** `/tests` — легаси (`useTestsStore` → `tests_*.php`),
   `/kiosk/tests` — второй (`src/tests/` → `forms*.php`). Не путать.

6. **Списки продублированы.** Разделы прав — в `backend/internal/auth/permissions.go`
   **и** `src/pages/Admin/portalSections.ts`. Категории курсов — в
   `permissions.go` **и** `src/pages/Courses/courseCategories.ts`. Реакции на
   новости, записи стены профиля и комментарии — в `backend/internal/handlers/news_reactions.go` **и**
   `src/composables/useNewsReactions.ts`. Окно поздравления с днём рождения (3 дня) — в
   `backend/internal/handlers/birthday_greetings.go` **и** `src/composables/useBirthdayGreetings.ts`.
   Менять парами.

7. **Страница скроллит себя, а не документ.** Корень — `h-dvh overflow-hidden`.
   Отсюда `h-full min-h-0 overflow-y-auto` в каркасе каждой страницы.

8. **Миграции идемпотентны.** `deploy.sh` прогоняет все `V*.sql` при каждом
   деплое. Новая миграция обязана быть повторно применимой.

9. **Тема настраивается пользователем.** 18 primary × 9 neutral × 4 шрифта ×
   5 радиусов × 5 масштабов × 2 схемы. Жёстко заданные цвета и размеры ломают
   настройку.

---

## Правила UI

Подробно — [`docs/design/`](docs/design/README.md). Коротко:

- Цвет — только семантические токены: `text-muted`, `bg-elevated`,
  `border-default`, `text-primary`. Не `text-green-600`.
- `color=` — только `primary | secondary | success | info | warning | error | neutral`.
- Кнопка по умолчанию нейтральная; `primary` — для главного действия экрана.
- Иконки — только Lucide (`i-lucide-*`) через `UIcon`.
- Панели и карточки — `rounded-panel` (следует за настройкой радиуса).
- Каркас страницы:
  ```html
  <UMain class="relative w-full h-full min-h-0">
    <div class="flex flex-col gap-6 w-full h-full min-h-0 max-w-[1600px] mx-auto
                overflow-y-auto scrollbar-hide p-px pb-8 *:shrink-0">
      <UPageHeader headline="…" title="…" description="…" />
  ```
- Создание и редактирование — `USlideover side="right"`; подтверждение —
  `UModal` с футером «Отмена» + действие.
- Пусто — `UEmpty variant="naked"` с иконкой, заголовком и подсказкой, что делать.
- Загрузка — `USkeleton` в форме будущего контента, не спиннер.
- Результат действия — toast через `useAppToast()`.
- Оптимистичное обновление **обязано** откатываться: проверять и `res.ok`,
  и `json.success`.
- Нет прав — контрол не рендерится (`v-if="canEditSection(...)"`), а не
  блокируется.
- Иконочная кнопка — `aria-label` + `UTooltip`; декоративная иконка — `aria-hidden="true"`.
- Порядок выбора: компонент проекта → компонент Nuxt UI → обёртка → новый
  компонент (с обоснованием).
- Не давать своим компонентам префикс `U`.

---

## Nuxt UI и MCP

Версия в проекте — **4.8.0** (Vue-режим, без Nuxt).

**MCP-сервер Nuxt UI в текущей сессии недоступен** — проверено по списку
инструментов сессии и реестру отложенных инструментов; конфигурации MCP в
репозитории нет.

Пока MCP недоступен, API компонентов проверять по установленной версии:

```
node_modules/@nuxt/ui/dist/runtime/components/<Component>.vue.d.ts   # props/emits/slots
node_modules/@nuxt/ui/dist/runtime/components/<Component>.vue        # разметка, имена слотов
node_modules/@nuxt/ui/dist/runtime/index.css                         # семантические переменные
```

Правила:

- Если MCP появится — использовать его, но **всё равно сверять с версией 4.8.0**.
- **Никогда не писать, что API проверен через MCP, если обращения не было.**
- Не выдумывать названия инструментов MCP, компонентов, пропсов и слотов.
- Официальная документация — только для 4.x, со сверкой по типам выше.

---

## Что делать с найденными проблемами

Нашли дефект вне рамок задачи → запись в
[`docs/known-issues.md`](docs/known-issues.md) с путём к файлу и строкой,
**без исправления**. Исправлять — отдельной задачей.

Известные проблемы безопасности ведутся в
[`docs/PROJECT_AUDIT.md`](docs/PROJECT_AUDIT.md). Закрытые там дыры
(SEC-001/006/007/008/009) заново не «чинить».
