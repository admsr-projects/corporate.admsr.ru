# Модули и пользовательские сценарии

Для каждого модуля: где страница, где логика данных, какие эндпоинты, что
проверить после изменения.

---

## Вход и сессия

**Сценарий.** Логин/пароль → `POST /api/auth.php` → профиль + `sessionToken` в
`localStorage`, httpOnly-cookie `corp_session` ставит API → редирект на `/welcome`
(если профиль неполный) или `/`.

| Что | Где |
|-----|-----|
| Экран | `src/pages/login.vue` (единственная форма с Zod-схемой и `UForm :schema`) |
| Клиентская сессия | `src/composables/useAuthSession.ts` |
| Фоновая активность / авто-логаут | `src/composables/useSessionActivity.ts` |
| Гвард | `src/router/index.js:210-294` |
| Онбординг | `src/composables/useOnboarding.ts`, `src/pages/OnboardingPage.vue` |
| Backend | `backend/internal/handlers/auth.go`, `backend/internal/auth/session.go` |

Эндпоинты: `auth.php`, `logout.php`, `check-auth.php`, `heartbeat.php`,
`session_bootstrap.php`.

**После изменения проверить:** вход, выход, поведение при истёкшей сессии,
`/welcome` для нового пользователя, что киоск по-прежнему открывается без входа.

Детали — [auth-and-permissions.md](auth-and-permissions.md).

---

## Рабочий стол

**Сценарий.** Приветствие → плитки сервисов → лента новостей → правая колонка
(календарь, дни рождения, журнал отсутствия).

| Что | Где |
|-----|-----|
| Страница | `src/pages/HomePage.vue` (693 строки) |
| Виджеты | `src/components/home/HomeNewsCard.vue`, `HomeCalendarWidget.vue`, `HomeEventsWidget.vue` («Мероприятия»: ближайшие предстоящие, без «Архива», до 5; ADR-055), `HomeAbsenceWidget.vue` |
| Виджет обучения | `src/pages/Courses/components/LearningHomeWidget.vue` |
| Лента | `src/composables/useNewsFeed.ts` + `src/stores/newsFeed.ts` |
| Плитки сервисов | `src/composables/usePortalServices.ts` |

Макет: при наличии контента правой колонки — `xl:grid-cols-[minmax(0,1fr)_420px]`,
иначе одна колонка с `max-w-4xl` (`HomePage.vue:388-393`). **[ПОДТВЕРЖДЕНО]**

---

## Новости

| Что | Где |
|-----|-----|
| Лента | `src/pages/News/NewsPage.vue` |
| Карточка | `src/pages/News/NewsDetailsPage.vue` |
| Данные (полный список) | `src/composables/useNewsData.ts` |
| Данные (курсорная лента) | `src/composables/useNewsFeed.ts`, `useCursorFeed.ts` |
| Реакции | `src/composables/useNewsReactions.ts` |
| Редактор (TipTap через `UEditor`) | `src/composables/newsEditor*.ts` (5 файлов) |

Эндпоинт: `news.php` (GET публичный; POST/PUT/DELETE — право секции `news`,
`backend/internal/handlers/news.go`).

Реакции (с 2026-09-28, ADR-033): 8 видов (`NEWS_REACTIONS` в
`useNewsReactions.ts` и `NewsReactionKeys` в `news_reactions.go` — менять
парами), таблица `news_reactions` (V11). У сотрудника на новость — несколько
разных реакций, каждая не больше одной. Компонент `src/components/NewsReactions.vue`
(карточка на рабочем столе, страница новости; в киоске скрыт). Ответы `news.php`
несут `reactions: [{key, count, mine}]`; `mine` — по сессии, без входа `false`.
Просмотр «уже засчитан в этой сессии» — по-прежнему `sessionStorage['news-viewed:v1']`.

Индекс под курсорную пагинацию: `db/migration/V8__news_feed_index.sql`.

### Комментарии (ADR-050)

| Что | Где |
|-----|-----|
| Данные и запросы | `src/composables/useNewsComments.ts` |
| Компоненты | `src/components/news/` — `NewsCommentsSection` (страница новости), `NewsCommentThread`, `NewsCommentItem`, `NewsCommentComposer` |
| Встроено | только `News/NewsDetailsPage.vue` (под текстом, `id="comments"`; не в киоске). На рабочем столе комментариев нет — кнопка «Прокомментировать» в карточке ленты ведёт на `/news/:id#comments` (прокрутка к разделу и курсор в поле) |
| Backend | `backend/internal/handlers/news_comments.go` (`news_comments.php`) |

Два уровня, как во ВКонтакте: ответ на ответ — в той же ветке с «в ответ Имя». Популярность — число
всех реакций, при равенстве выше более новый. На странице новости — все, «Популярные» / «Новые», по 20
с «Показать ещё». В ветке без «Развернуть» виден самый популярный ответ; развёрнутая — по времени.
Реакции — те же 8 ключей, `NewsReactions target="comment"`. Ответ на ваш комментарий — в колокольчик,
клик ведёт на `/news/:id?comment=<id>`: ветка закрепляется сверху развёрнутой и подсвечивается.

### Фильтр мата (ADR-053)

| Что | Где |
|-----|-----|
| Поиск слов | `backend/internal/profanity/profanity.go` (+ `profanity_test.go`) — список только здесь |
| Задача и токен | `backend/internal/handlers/profanity_gate.go` — `profanityGate(...)` перед сохранением текста |
| Фронт | `src/composables/useProfanityGate.ts` (`postWithProfanityGate`), окно `src/components/ProfanityChallengeModal.vue` в `App.vue` |
| Где действует | комментарии (`create`/`update`), стена профиля (`create`/`update`/`greet`) |

Текст с матом — 422 с задачей; решили — публикуется со звёздочками (`х****й`). Добавляя новое место с
пользовательским текстом, вызовите `profanityGate` в Go и отправляйте через `postWithProfanityGate`.

---

## Мероприятия

| Что | Где |
|-----|-----|
| Афиша | `src/pages/Events/EventsPage.vue` |
| Карточка | `src/pages/Events/EventDetailsPage.vue` |
| Данные | `src/composables/useEventsData.ts` |

Эндпоинт: `events.php` (GET публичный, мутации — секция `events`).
Связь с альбомом галереи: `events.album_id` — `db/migration/V7__events_gallery_album.sql`.

Запись на мероприятие («Записаться») хранится на сервере: `event_rsvp.php` (`calendar_personal.go`,
таблица `event_rsvps`, V16), состояние общее для страниц — `src/composables/useEventRsvp.ts`. Прежние отметки
из localStorage (`events-rsvp:v1`) не переносились и удаляются при загрузке модуля.

**Время, место, лимит мест, записавшиеся (V22, ADR-054).** Поля «Начало», «Окончание», «Место», «Лимит мест»
в создании и правке — `src/pages/Events/components/EventScheduleFields.vue` (время — `UInputTime`, 24 ч).
Чтение, подписи и тело запроса — `src/composables/eventSchedule.ts`. У старых мероприятий без полей время и место
по-прежнему угадываются из текста описания (афиша, календарь). Лимит заполнен — «Мест нет», сервер отвечает 409.
Карточка «Записавшиеся» на странице мероприятия — только организаторам (правка раздела «Мероприятия»):
`event_rsvp.php?eventId=`. Время и место мероприятия видны и в календаре, и в напоминании накануне.

**Несколько времён (V23, ADR-056).** У мероприятия одно или несколько времён в один день, у каждого свой
лимит; сотрудник записывается на одно (другое — «Перезаписаться»). Форма — строки времени с лимитом и
«+ Добавить время» под последней (`EventScheduleFields.vue`; удаление времени с записями — через подтверждение).
Карточка в афише: одно время — кнопка, несколько — `UDropdownMenu` с временами и местами. Страница мероприятия —
«Выберите время»; у организатора записавшиеся сгруппированы по времени. Календарь, виджет и напоминание накануне
показывают время, на которое записан сотрудник. Расчёты — `eventSchedule.ts` (`slots`, `slotTimeLabel`,
`isSlotFull`), запись — `useEventRsvp.ts` (`joinedSlot`, `setJoined(eventId, join, slotId)`).

**Календарь, виджет, описание (ADR-055).** В календаре — только мероприятия, на которые сотрудник записался;
новые видны в афише и в виджете «Мероприятия» на рабочем столе (`src/components/home/HomeEventsWidget.vue`).
Описание — HTML из того же редактора, что у новостей (`src/pages/Events/components/EventDescriptionEditor.vue`);
показ — `v-html` с `newsEditorHtmlClass`, превью и поиск времени/места — по тексту без разметки
(`eventDescriptionText` в `eventSchedule.ts`). Старые текстовые описания показываются как текст с переносами,
в редакторе открываются абзацами.

Генерация WebP-обложек: `npm run images:webp` → `scripts/generate-events-webp.mjs`.

---

## Фотогалерея

| Что | Где |
|-----|-----|
| Альбомы | `src/pages/Gallery/GalleryPage.vue` |
| Альбом | `src/pages/Gallery/GalleryAlbumPage.vue` |
| Данные | `src/composables/useGalleryData.ts` |

Эндпоинты: `gallery.php`, `gallery_base.php` (мутации — секция `gallery`,
`backend/internal/handlers/gallery.go:109,146,204,436,506`).

---

## Календарь

| Что | Где |
|-----|-----|
| Страница | `src/pages/CalendarPage.vue` (1141 строка) |
| Агрегатор источников | `src/composables/useCalendarFeed.ts` |

Пять источников (`CalendarSource`): `event`, `meeting`, `birthday`, `learning`,
`personal` — `useCalendarFeed.ts:5`. Личные записи (`personal`) и встречи
пользователя хранятся на сервере: `calendar_entries.php` (`calendar_personal.go`, таблица
`calendar_entries`, V16), видны только их автору. Прежние записи из localStorage
(`portal-calendar-local:v1`) не переносились и удаляются при загрузке модуля.

Личные события и встречи (ADR-051): цвет (`EventColorPicker.vue` — «По умолчанию», 8 базовых цветов,
палитра `UColorPicker`; хранится в `calendar_entries.color`; красят общие функции `calendarItem*` из
`useCalendarFeed.ts` — и страница календаря, и виджет на рабочем столе), меню «⋮» — «Изменить» (та же форма, что
при создании) и «Удалить»; кнопки «Открыть» у них нет. Напоминание накануне — в колокольчике: о личных
событиях и о мероприятиях, куда записались; создаёт его Go при запросе уведомлений (планировщика нет),
клик ведёт на `/calendar?date=YYYY-MM-DD` (страница открывает этот день) или на страницу мероприятия.

Единственное место в проекте, где брейкпоинты читаются из JS:
`useMediaQuery('(min-width: 1024px)')` и `768px` — `CalendarPage.vue:38-39`.

---

## Журнал отсутствия

| Что | Где |
|-----|-----|
| Страница | `src/pages/AbsenceJournalPage.vue` (1992 строки — самый крупный файл фронта) |
| Флаг «есть активное отсутствие» | `src/stores/absenceJournal.js` (localStorage `absence-journal:has-active`, синхронизация между вкладками через событие `storage`) |
| Backend | `backend/internal/handlers/absence.go` |

Эндпоинт: `absence_journal.php`. GET — любой авторизованный (`requireUser`),
POST/PUT/DELETE — редактор секции `absence_journal` за любого; обычный сотрудник — только
свои записи (`DELETE` — только свою активную). См. SEC-009 в `docs/PROJECT_AUDIT.md`.

Мутации идут через `apiSessionFetch` (Bearer), а контролы правки гейтятся
`canEditSection('absence_journal')`.

**Данные сотрудника в журнале** (с 2026-09-28, ADR-032): ФИО, ОФО и должность
берутся из `user_info` — те же, что в профиле и админке. POST берёт их с сервера
(поля `fio`/`ofo`/`role` из тела игнорируются); GET отдаёт текущие данные
сотрудника через `LEFT JOIN user_info`, копия в записи — запасной вариант.
Названия ОФО — из дерева `ofo_unit` (`useOfoTree`), не из легаси `ofo.php`.

---

## Профиль и стена

| Что | Где |
|-----|-----|
| Страница | `src/pages/ProfilePage.vue` — `/profile` (своя), `/profile/:id` (коллеги) |
| Редактирование | `src/components/profile/ProfileEditSlideover.vue` — аватар (готовый или своё фото), «О себе», «Интересы» |
| Желания, награды | `src/components/profile/ProfileWishes.vue`, `ProfileAwards.vue` |
| Плашка-заголовок блока | `src/components/profile/ProfileSection.vue` |
| Отображаемое имя/аватар в шапке | `src/composables/useProfileDisplay.ts`, `useHeaderUser.ts` |
| Стена | `src/composables/useProfileWall.ts`, `src/components/profile/ProfileWallPost.vue` |
| Реакции на записи | `NewsReactions.vue` с `target="wall"`, `useNewsReactions.ts` (`useReactions`) |
| Аватары по умолчанию | `src/constants/profileAvatars.ts` |
| API | `profile.php` (`?view=page`), `profile_wall.php`, `profile_extras.php`, `profile_avatar.php` — `backend/internal/handlers/profile.go`, `profile_wall.go`, `profile_extras.go`, `profile_avatar.go`; обработка фото — `backend/internal/media/webp.go` (`SaveAvatar`) |

Компоновка старого ВКонтакте на токенах портала (ADR-040): слева аватар,
действия и «Коллеги» (то же подразделение), справа ФИО, анкета (должность,
подразделение, день рождения, телефон, почта), «Информация» («О себе»,
«Интересы»), награды, пройденные курсы, журнал отсутствия и стена; слева ещё
«Желания». ФИО, телефон, почта, подразделение и должность в профиле **не
редактируются** (ADR-041). Писать на стене может любой вошедший сотрудник; запись —
простой текст до 4000 символов, выводится как текст (не `v-html`). Попасть в
профиль коллеги — из поиска портала (Ctrl+K → «Сотрудники»), из «Коллег» и по
имени автора записи.

---

## Дни рождения

| Что | Где |
|-----|-----|
| Данные | `src/composables/useBirthdayColleagues.ts` |
| Backend | `backend/internal/handlers/birthdays.go` |

Эндпоинт: `birthdays.php`. Источник данных — `*.xlsx` в каталоге `BIRTHDAYS_DIR`
или `${UPLOAD_DIR}/birthdays_xlsx` (`birthdays.go:51`), парсинг через
`xuri/excelize`. Право на редактирование — секция `birthdays`. Файлы лежат на
диске той машины, где запущен Go API: локально без них дней рождения нет.

### Поздравления (ADR-049)

| Что | Где |
|-----|-----|
| Состояние, окно, шаблоны | `src/composables/useBirthdayGreetings.ts` |
| Форма | `src/components/BirthdayGreetingSlideover.vue` |
| Кнопки | `HomePage.vue` (виджет «Дни рождения коллег», группа «Сегодня»), `CalendarPage.vue` (панель дня, клик по дню рождения) |
| Backend | `backend/internal/handlers/birthday_greetings.go` (`profile_wall.php`, `action=greet` / `my_greetings`) |

Поздравление — запись на стене именинника. Кнопка «Поздравить» есть, только если ФИО из xlsx
сопоставилось с активной учётной записью, это не вы и сегодня — день рождения или до 3 дней после.
После отправки — «Вы поздравили» со ссылкой на стену. Окно (`BIRTHDAY_GREET_WINDOW_DAYS` /
`birthdayGreetWindowDays`) задано и во фронте, и в Go — менять парами; решает сервер.

## Уведомления

| Что | Где |
|-----|-----|
| Состояние | `src/composables/useNotifications.ts` |
| Колокольчик | `src/components/NotificationsBell.vue` (в `AppHeader.vue`) |
| Backend | `backend/internal/handlers/notifications.go` (`notifications.php`) |

Виды: запись на вашей стене (`wall_post`), поздравление с днём рождения (`birthday_greeting`).
Создаются в `profile_wall.php` при `create` / `greet`, если пишете не себе. Удалили запись — ушло и
уведомление. Опроса по таймеру нет: обновление при открытии колокольчика, переходах (не чаще
раза в 30 с) и возврате на вкладку — иначе фоновые запросы отменили бы выход по бездействию.

---

## Сервисы

| Что | Где |
|-----|-----|
| Страница-каталог | `src/pages/ServicesPage.vue` (544 строки, включая админ-редактор) |
| Данные | `src/composables/usePortalServices.ts` |
| Каталог внутренних страниц | `src/pages/Services/portalServiceCatalog.ts` |
| Backend | `backend/internal/handlers/portal_services.go` |
| Таблица | `db/migration/V10__portal_services.sql` |

Сервис бывает `internal` (путь внутри SPA) или `external` (внешний URL).
Включённые внутренние сервисы становятся детьми пункта «Сервисы» в сайдбаре —
`usePortalNavigation.ts:useSidebarNavItems`.

Эндпоинт `portal_services.php` существует **только в Go**; PHP-реализации нет.
При недоступном API используется `FALLBACK_PORTAL_SERVICES` (журнал отсутствия,
формы, заявки) — `usePortalServices.ts:30-64`. **[ПОДТВЕРЖДЕНО]**

---

## Формы и тесты — две параллельные реализации

Это самое неочевидное место проекта. **[ПОДТВЕРЖДЕНО]**

### A. Легаси-модуль — то, что видит сотрудник на `/tests`

| Что | Где |
|-----|-----|
| Точка входа | `src/pages/TestsBlankPage.vue` (маршрут `/tests`) |
| Конструктор / прохождение / статистика | `src/pages/Tests/TestBuilder.vue`, `TestRunner.vue`, `StatsDetail.vue`, `QuestionsBuilder.vue`, `StatChart.vue` |
| Модель формы | `src/pages/Tests/testForm.ts` (числовой `id`) |
| Типы вопросов | `src/pages/Tests/questionTypes.ts` — 11 типов: `single`, `multiple`, `dropdown`, `text`, `textarea`, `scale`, `yesno`, `number`, `date`, `match` (соответствие), `classify` (классификация); редактор последних двух — `src/pages/Tests/components/PairingEditor.vue` |
| Стор | `src/composables/useTestsStore.ts` |
| Публичная ссылка | `src/pages/TestLinkPage.vue` (`/t/:token`) |
| Агрегация статистики | `src/composables/useTestStats.ts` |
| Backend | `backend/internal/handlers/tests_routes.go`, `backend/internal/tests/` |

Эндпоинты: `tests_list/save/publish/unpublish/delete/direct/submit/stats/participant/by_token.php`.

Клиент до сих пор кладёт `userId` в тело запроса (`useTestsStore.ts:37,53,…`),
но сервер эту величину **игнорирует** и берёт личность только из сессии
(исправление SEC-008). Рудимент не удалён. **[ПОДТВЕРЖДЕНО]**

### B. Второй модуль — доступен только на `/kiosk/tests`

| Что | Где |
|-----|-----|
| Точка входа | `src/pages/TestsPage.vue` (маршрут `/kiosk/tests`) |
| Компоненты | `src/components/tests/FormBuilder.vue`, `FormPreview.vue`, `FormTake.vue`, `FormReport.vue`, `FormsList.vue`, `QuestionEditor.vue` |
| Типы | `src/tests/types.ts` (UUID) |
| Zod-схемы | `src/tests/schemas.ts` (используются в `FormBuilder.vue:5`) |
| API-обёртки | `src/tests/api.ts` |
| Pinia-стор | `src/tests/store.ts` |
| Backend | `backend/internal/handlers/forms.go` |

Эндпоинты: `forms.php`, `forms_list/publish/submit/report/archive/delete.php`.
Таблицы `forms`, `questions`, `question_options`, `form_responses`,
`response_answers` (V1), отдельные от `test_forms`/`test_questions` (V2).

> Роль каждого модуля и планы по их объединению в коде не зафиксированы.
> **[НЕИЗВЕСТНО]** — вопрос владельцу, см. [known-issues.md](known-issues.md).

---

## Учебные курсы (LMS)

| Что | Где |
|-----|-----|
| Админ-страницы | `src/pages/Courses/admin/` (12 экранов) |
| Страницы сотрудника | `src/pages/Courses/employee/` (5 экранов) |
| Общие компоненты | `src/pages/Courses/components/` |
| Стор | `src/composables/useCoursesStore.ts` (565 строк) |
| Сертификат PDF | `src/pages/Courses/certificatePdf.ts` (jsPDF) |
| Готовность курса | `src/pages/Courses/courseReadiness.ts` |
| Следующее действие | `src/pages/Courses/followNextAction.ts` |
| Категории | `src/pages/Courses/courseCategories.ts` (2 значения) |
| Крошки | `src/pages/Courses/useAdminCoursePortalBreadcrumbs.ts` |
| Backend | `backend/internal/handlers/courses.go`, `attempt_review.go`, `backend/internal/courses/` |

Иерархия: курс → версия → темы → материалы / тесты; назначение создаёт
enrollment; прохождение с последовательной разблокировкой и снимком в
`course_completions`. Подробно — [courses-architecture.md](courses-architecture.md).

Экспорт результатов в Excel — `CourseResultsPage.vue` (SheetJS).
Флаг генерации сертификата — `db/migration/V9__course_certificate.sql`.

Smoke: `npm run test:courses`.

---

## Администрирование

| Что | Где |
|-----|-----|
| Дэшборд | `src/pages/Admin/AdminDashboardPage.vue` (маршрут `/admin`, `requiresAdmin`) |
| Разделы прав | `src/pages/Admin/portalSections.ts` |
| Панель ОФО | `src/components/AdminOfoPanel.vue` |
| Пользователи | `src/composables/useUsersData.ts` |
| Группы доступа | `src/composables/useGroupsData.ts` |
| ОФО | `src/composables/useOfoTree.ts`; компоненты `OfoSelect.vue`, `OfoMultiSelect.vue` |

Эндпоинты: `users.php` (только админ), `portal_groups.php`, `ofo*.php`.

### Девблог (ADR-052)

| Что | Где |
|-----|-----|
| Рабочая зона | `src/pages/Admin/DevblogPage.vue` (маршрут `/admin/devblog`, `requiresAdmin`; пункт «Редактировать девблог» в меню профиля — только в роли администратора) |
| Окошко при входе | `src/components/DevblogWelcomeModal.vue` (в `App.vue`, только обычный интерфейс) |
| Запросы | `src/composables/useDevblog.ts` |
| Версия выпуска | поле «Версия» под заголовком (по умолчанию последняя + 0.0.1, первая — 1.0.0, «X.Y.Z»); `src/components/DevblogVersionBadge.vue` — под заголовком на странице новости-девблога (`news.php?id=` отдаёт `devblogVersion`), в окошке и предпросмотре, основной цвет темы с переливом |
| Поле Markdown | `src/components/MarkdownEditor.vue` + `src/composables/markdownEditing.ts` (панель, горячие клавиши, списки по Enter/Tab, ссылка из вставки) |
| Стандартная обложка | `public/devblog-cover.svg` (`public/img/` — загрузки, вне git) |
| Backend | `backend/internal/handlers/devblog.go` (`devblog.php`) |
| Где виден | лента рабочего стола (автор — «Разработчики портала»), колокольчик (тоже «Разработчики портала», без аватара), страница новости. Во вкладке «Новости» девблогов нет (`news.php?excludeCategory=Девблог`). «По теме» и «Предыдущая / Следующая» у девблога — только девблоги, у новости — без девблогов. Категория и подпись — `DEVBLOG_CATEGORY`, `DEVBLOG_AUTHOR` в `useDevblog.ts` |

Один общий черновик в Markdown: слева текст, справа предпросмотр (`UEditor` с `content-type="markdown"`,
только чтение). Автосохранение через 1,5 с с номером версии: сохранил другой администратор — выбор «Взять
сохранённую / Оставить мою». «Опубликовать» отправляет HTML, построенный тем же редактором, — сервер
создаёт новость категории «Девблог» (лента, реакции, комментарии — как у новостей), уведомление `devblog`
всем активным сотрудникам (кроме автора) и очищает черновик. Окошко — последний опубликованный девблог,
который сотрудник не закрыл; закрыли — отметка на сервере, уведомление о нём прочитано.

BUG-001/BUG-002 (откат оптимистичных обновлений при ошибке сервера) исправлены —
см. [PROJECT_AUDIT.md](PROJECT_AUDIT.md).

---

## Глобальный поиск

| Что | Где |
|-----|-----|
| Логика | `src/composables/usePortalGlobalSearch.ts` (436 строк) |
| Подключение | `src/App.vue:19-28` (`UDashboardSearch`) |
| Горячая клавиша | `src/App.vue:76-83` — Ctrl/⌘+K по `e.code === 'KeyK'`, работает на русской раскладке |

Группы результатов: «Разделы» (статический список), «Сотрудники», «Новости»,
«Мероприятия», «Фотогалерея», «Формы», «Сервисы», «Обучение», «Документация».
Фильтрация на стороне Fuse.js (`:fuse="{ resultLimit: 48 }"`), группы помечены
`ignoreFilter: true` — фильтрацию делает сам composable.

> `UDashboardSearch` получает `shortcut="ctrl_shift_alt_f12"` (`App.vue:21`) —
> встроенный шорткат намеренно «уведён» в нереальную комбинацию, чтобы работал
> собственный обработчик Ctrl+K. Комментария об этом в коде нет. **[ПОДТВЕРЖДЕНО]**

---

## Киоск

| Что | Где |
|-----|-----|
| Layout | `src/AppKiosk.vue` |
| Шапка / подвал | `src/components/KioskHeader.vue`, `KioskAside.vue` |
| Главная | `src/pages/Kiosk/KioskHomePage.vue` |
| Авто-тема | `src/composables/useColorModeSchedule.ts` |
| Скрипты запуска на Windows | `kiosk/*.cmd` |

Холст: `min(1080px,100vw) × min(1920px,100vh)`, `aspect-9/16` (`AppKiosk.vue:3`).
Тема по расписанию: 00:00–14:59 светлая, 15:00–23:59 тёмная
(`useColorModeSchedule.ts:8-10`).

Ограничения: авторизация не требуется; роль принудительно `user`;
`requiresAdmin` / `requiresSection` внутри киоска → редирект на `/kiosk`
(`router/index.js:266-277`).

> По решению владельца (PROJECT_AUDIT §8) киоск **не трогаем**.

---

## Загрузка файлов

| Что | Где |
|-----|-----|
| Изображения | `POST /api/Upload/upload.php` → `backend/internal/handlers/gallery.go` (`Upload`), `backend/internal/media/webp.go` |
| Материалы курсов | `POST /api/course_materials_upload.php` → `coursesH.MaterialsUpload` |

Каталоги: `${UPLOAD_DIR}/img/FullPic`, `${UPLOAD_DIR}/img/SmallPic`,
`${UPLOAD_DIR}/courses/{courseId}/`.

Известное отличие Go от PHP: изображения сохраняются как JPEG, без libwebp
(`backend/README.md`, раздел «Известные отличия»). **[ПОДТВЕРЖДЕНО]**

Выдача файлов курса через `GET /api/course_file.php` — **только PHP**, в Go-mux
эндпоинта нет (`backend/cmd/api/main.go`). **[ПОДТВЕРЖДЕНО]**
