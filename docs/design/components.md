# Компоненты

Порядок выбора при любой UI-задаче:

```
1. Компонент проекта (список ниже)
2. Компонент Nuxt UI 4.8.0 (инвентарь — nuxt-ui.md)
3. Обёртка над компонентом Nuxt UI в проекте
4. Новый компонент — только если 1–3 не подходят, с обоснованием
```

---

## Компоненты проекта

`src/components/` — 38 файлов (включая подкаталоги `home/`, `news/`, `profile/`, `tests/`).
**[ПОДТВЕРЖДЕНО]**

### Каркас

| Компонент | Назначение | Строк |
|-----------|------------|-------|
| `AppAside.vue` | Боковое меню основного портала (`UDashboardSidebar`) | 114 |
| `AppHeader.vue` | Шапка: крошки, поиск, уведомления, меню профиля (`UDashboardNavbar`) | 416 |
| `KioskAside.vue` | Навигация киоска — крупные плитки | 33 |
| `KioskHeader.vue` | Шапка киоска | 99 |

### Общего назначения

| Компонент | Назначение | Где используется |
|-----------|------------|------------------|
| `SectionInDevelopment.vue` | Полноэкранная заглушка «Раздел в разработке». Пропсы `title`, `description` | `PersonnelReservePage`, `DevelopmentMotivationDepartmentPage` |
| `ScrollToTopButton.vue` | Плавающая кнопка «Наверх» для страниц, скроллящих свой контейнер. Primary-кнопка с подписью, закреплена в правом нижнем углу окна (`fixed`: `UMain` ограничен 1600 px, внутри него кнопка ложилась на контент). Проп `target` — элемент-скроллер, `threshold` (по умолчанию 600 px). Учитывает `prefers-reduced-motion` | `HomePage`, `News/NewsPage`, `Events/EventsPage`, `Gallery/GalleryPage`, `Gallery/GalleryAlbumPage`, `AbsenceJournalPage` |
| `UserWorkFields.vue` | Место работы: ОФО (`OfoSelect`) + должность из справочника ОФО. `v-model:ofo-id`, `v-model:role` (строка, как `user_info.role`). Должность не из справочника не стирается — показывается с пометкой. При смене ОФО должность сбрасывается | `ProfilePage`, `OnboardingPage`, `Admin/AdminDashboardPage` (форма пользователя) |
| `OfoSelect.vue` | Выбор одного подразделения ОФО | формы, `TestLinkPage` |
| `OfoMultiSelect.vue` | Выбор нескольких подразделений | `TestsBlankPage`, адресация форм |
| `AdminOfoPanel.vue` | Дерево ОФО в админке. Клик по подразделению — `USlideover` с путём, счётчиками, сотрудниками (переключатель «С вложенными»), вложенными подразделениями и должностями. Событие `open-user` — открыть карточку сотрудника | `AdminDashboardPage` |
| `BirthdayGreetingSlideover.vue` | «Поздравить с днём рождения» — `USlideover`: именинник, 5 готовых фраз (подставляются в поле), текст до 4000 символов, «Отмена» + «Поздравить». Без пропсов: открывается через `useBirthdayGreetings().openGreeting(...)`, один экземпляр на странице (ADR-049) | `HomePage`, `CalendarPage` |
| `DevblogCover.vue` | Обложка девблога между версией и текстом: картинка целиком (`object-contain`) на размытой подложке из неё же — как фото на странице новости, своя обложка не обрезается. Пропсы `src`, `alt` | `DevblogWelcomeModal`, `Admin/DevblogPage` (предпросмотр) |
| `DevblogVersionBadge.vue` | «Версия 1.0.1» под заголовком девблога: основной цвет темы (`--ui-primary`) с бегущим переливом (градиент по тексту, `background-clip: text`); при `prefers-reduced-motion` — без анимации. Проп `version` | `NewsDetailsPage`, `DevblogWelcomeModal`, `Admin/DevblogPage` |
| `DevblogWelcomeModal.vue` | Окошко девблога при первом заходе после публикации: обложка, текст (HTML из Markdown), реакции, поле комментария, «Все комментарии» / «Закрыть». Закрытие любым способом отмечается на сервере (ADR-052) | `App.vue` |
| `ProfanityChallengeModal.vue` | Окно задачи для текста с матом: сообщение о неверном ответе, задача, поле «Ответ», «Опубликуется так:» с текстом под звёздочками; «Отмена» / «Опубликовать». Состояние — `profanityState` из `useProfanityGate.ts`, открывает `postWithProfanityGate` (ADR-053) | `App.vue` |
| `MarkdownEditor.vue` | Поле Markdown с помощниками в духе Obsidian: панель (H1–H3, жирный, курсив, зачёркнутый, код, списки, цитата, блок кода, ссылка), горячие клавиши по физической клавише (Ctrl+B / I / E, Ctrl+Shift+X / 7 / 8 / 9 / K — Ctrl+K занят поиском портала), Enter продолжает список или цитату (на пустом пункте — выход), Tab / Shift+Tab — вложенность (в обычном тексте Tab уводит фокус), вставка адреса поверх выделения — ссылка, счётчик слов. Правки через `execCommand('insertText')` — работает Ctrl+Z. Логика — чистые функции `src/composables/markdownEditing.ts`. `v-model` — строка | `Admin/DevblogPage` |
| `EventColorPicker.vue` | Цвет личного события: «По умолчанию» (цвет по типу), 8 базовых цветов, «Другой цвет» — `UPopover` с `UColorPicker`. `v-model` — `#rrggbb` или `null`. Цвета — данные пользователя, поэтому hex, а не токены темы (исключение из правила токенов, ADR-051); текст события остаётся в цветах темы | `CalendarPage` (форма события) |
| `NotificationsBell.vue` | Колокольчик в шапке: `UChip` с числом непрочитанных, `UPopover` со списком (аватар автора, вид, отрывок, время), «Прочитать все», клик — отметить и открыть свою стену. Обновляется при открытии, переходах и возврате на вкладку, **без таймера** (ADR-049) | `AppHeader` |

### Рабочий стол

| Компонент | Назначение |
|-----------|------------|
| `home/HomeNewsCard.vue` | Карточка новости в ленте рабочего стола. Открывает новость клик по фото (кроме кнопок «Скачать» / «Открыть» — они останавливают всплытие) и по заголовку с текстом (`RouterLink`, работает с клавиатуры и в новой вкладке); кнопка «Прокомментировать» (была «Подробнее») ведёт на `/news/:id#comments` — страница новости прокручивается к комментариям и ставит курсор в поле; шапка с автором и «Подписаться» и реакции — свои действия |
| `home/HomeCalendarWidget.vue` | Виджет ближайших событий (мероприятия — только те, на которые записались) |
| `home/HomeEventsWidget.vue` | «Мероприятия» на рабочем столе: ближайшие предстоящие (до 5, без «Архива») — дата, время, место, «Записалось N из M», «Вы записаны» / «Мест нет»; строка ведёт на страницу мероприятия. Вид — как у «Календаря» и «Дней рождения коллег» (ADR-055) |
| `home/HomeAbsenceWidget.vue` | Виджет журнала отсутствия |

### Комментарии к новостям (`news/`, ADR-050)

| Компонент | Назначение |
|-----------|------------|
| `news/NewsCommentsSection.vue` | Страница новости: заголовок со счётчиком, `UTabs` «Популярные / Новые» (компактный переключатель), поле, ветки, «Показать ещё», пусто — `UEmpty naked`; закреплённая ветка из уведомления |
| `news/NewsCommentThread.vue` | Ветка: комментарий + самый популярный ответ, «Развернуть ещё N ответов» / «Свернуть», поле ответа, подтверждение удаления `UModal` |
| `news/NewsCommentItem.vue` | Комментарий как запись стены: аватар, имя, «в ответ …», текст, время, «Ответить / Изменить / Удалить», реакции; правка на месте |
| `news/NewsCommentComposer.vue` | `UTextarea` (autoresize) + иконочная кнопка отправки, Ctrl+Enter, для ответа — «Отмена» |

### Профиль

| Компонент | Назначение |
|-----------|------------|
| `profile/ProfileWallPost.vue` | Запись на стене: автор, текст, реакции, правка на месте, «Удалить» |
| `profile/ProfileSection.vue` | Блок профиля с плашкой-заголовком (`bg-elevated`), как «Информация» / «Стена» у старого ВКонтакте |
| `profile/ProfileEditSlideover.vue` | «Редактировать страницу» — `USlideover`: аватар (готовый или загруженное фото), «О себе», «Интересы» (ФИО, контакты, ОФО, должность — не редактируются) |
| `profile/ProfileWishes.vue` | «Желания» сотрудника: список, добавление и удаление — только у владельца |
| `profile/ProfileAwards.vue` | «Награды»: список для всех, «Выдать награду» и удаление — только администратору |

### Формы — второй модуль (только `/kiosk/tests`)

`components/tests/`: `FormBuilder.vue`, `FormPreview.vue`, `FormTake.vue`,
`FormReport.vue`, `FormsList.vue`, `QuestionEditor.vue`.

> **Не переиспользовать для основного портала.** `/tests` работает на другом
> наборе — `src/pages/Tests/*`. См. [modules.md](../modules.md).

### Формы — легаси-модуль (основной портал `/tests`)

`src/pages/Tests/`: `TestBuilder.vue`, `TestRunner.vue`, `StatsDetail.vue`,
`QuestionsBuilder.vue`, `StatChart.vue`, `ModalTextField.vue`,
`components/TestSettingsForm.vue`.

### Обучение

`src/pages/Courses/components/`: `CourseStatusBadge.vue`, `CourseTestEditor.vue`,
`LearningHomeWidget.vue`, `MyCourseCard.vue`.

> `MyCourseCard.vue` добавлен 2026-09-22 в незакоммиченной работе — состояние
> уточняйте по коду.

---

## Автоимпорт

Компоненты подключаются **без явного импорта**: плагин `@nuxt/ui/vite`
использует `unplugin-vue-components` и генерирует `components.d.ts`. Там же
перечислены компоненты проекта и Nuxt UI, попавшие в последнюю сборку.
Композаблы Nuxt UI (`useToast`, `defineShortcuts`, …) — через
`unplugin-auto-import` (`auto-imports.d.ts`). **[ПОДТВЕРЖДЕНО]**

Оба файла генерируемые. Не редактировать вручную.

---

## Паттерны использования Nuxt UI

Цифры — снимок вхождений в `src/` на 2026-09-22. Пересчитать:
`grep -rho 'variant="[a-z]*"' src/ --include=*.vue | sort | uniq -c | sort -rn`

### Кнопки

| Проп | Распределение |
|------|---------------|
| `color` | `neutral` 279, `primary` 90, `error` 49, `warning` 29 |
| `variant` | `subtle` 104, `outline` 103, `ghost` 74, `soft` 66, `solid` 18, `link` 15 |
| `size` | `lg` 108, `md` 105, `xl` 94, `sm` 75, `xs` 41 |

Правила, выводимые из кода:

- **По умолчанию кнопка нейтральная.** `color="primary"` — только для главного
  действия экрана или формы.
- Иконочная кнопка: `square` + **обязательный** `aria-label`.
- Контролы шапки: `color="neutral" variant="outline" size="md" class="h-8"`.
- Кнопки в футере модальных окон и панелей: `size="xl"`.
- Отправка формы: `<UButton type="submit" block :loading="…">`.

### Карточки

| Компонент | Когда |
|-----------|-------|
| `UCard` | Контейнер с телом/шапкой/футером. Варианты: `soft` 7, `outline` 3, `subtle` 1 |
| `UPageCard` | Кликабельная карточка-ссылка (`:to`), плитки сервисов, навигация «пред./след.» |

Плитка сервиса (`HomePage.vue:413-425`):

```html
<UPageCard
  :title="svc.label" :icon="svc.icon" :to="svc.to"
  :target="svc.external ? '_blank' : undefined"
  :rel="svc.external ? 'noopener noreferrer' : undefined"
  :external="svc.external || undefined"
  variant="soft" class="bg-elevated"
/>
```

Внешние ссылки всегда получают `rel="noopener noreferrer"`. **[ПОДТВЕРЖДЕНО]**

### Модальные окна — для подтверждений

`UModal` (~20 файлов). Паттерн из `AdminDashboardPage.vue:1705-1716`:

```html
<UModal v-model:open="groupDeleteOpen"
        title="Удалить группу?"
        description="Участники потеряют права этой группы. Действие необратимо.">
  <template #footer>
    <div class="flex justify-end gap-3 w-full">
      <UButton color="neutral" variant="outline" size="xl" @click="…">Отмена</UButton>
      <UButton color="error" size="xl" :loading="…">Удалить</UButton>
    </div>
  </template>
</UModal>
```

Правила:
- `title` — вопрос; `description` — последствие, включая «Действие необратимо».
- Футер: `flex justify-end gap-3 w-full`, `size="xl"`.
- Отмена слева (`neutral outline`), деструктивное действие справа (`error`).
- Открытие — `v-model:open`.

### Боковые панели — для создания и редактирования

`USlideover` (~12 файлов). Паттерн из `NewsPage.vue:410-418`:

```html
<USlideover v-model:open="createOpen" side="right"
            title="Новая новость" description="Заполните данные новости"
            :ui="{ content: '!max-w-full sm:!max-w-2xl lg:!max-w-4xl xl:!max-w-5xl' }">
  <template #body>
    <UForm :state="createState" class="space-y-4" @submit.prevent="handleCreateSubmit">
      <UFormField label="Название" name="title" required>
        <UInput v-model="createState.title" size="xl" />
      </UFormField>
      …
```

Правила:
- `side="right"` всегда.
- Ширина растёт по брейкпоинтам через `:ui.content` (`!max-w-*`).
- Поля внутри панели — `size="xl"`.
- Вертикальный ритм формы — `space-y-4`.

Вспомогательные стили выпадающих элементов внутри панелей вынесены в
`src/composables/slideoverFieldUi.ts` (`slideoverSelectContent`,
`slideoverPopoverContent`). **[ПОДТВЕРЖДЕНО]**

### Формы

- `UForm` + `UFormField` (`label`, `name`, `required`) + контрол.
- **Zod-схема применяется только на экране входа** (`:schema`, `login.vue:98`).
  Остальные формы валидируются вручную — см.
  [states-and-feedback.md](states-and-feedback.md).
- Вертикальный ритм: `space-y-4` в панелях, `flex flex-col gap-4` в карточках.
- Контролы растягиваются `class="w-full"`.

Используемые контролы: `UInput`, `UTextarea`, `USelect`, `USelectMenu`,
`UCheckbox`, `UCheckboxGroup`, `URadioGroup`, `USwitch`, `UInputDate`,
`UInputTime`, `UFileUpload`, `UEditor`.

### Таблицы

`UTable` — 4 файла: `AdminDashboardPage.vue`, `AbsenceJournalPage.vue`,
`components/tests/FormsList.vue`, `components/tests/FormReport.vue`.

Для более простых табличных данных используются grid-раскладки с подписями
колонок `text-xs text-muted` (например `AdminDashboardPage.vue:1579-1583`).

### Вкладки

`UTabs` в двух ролях:
1. **Навигация по разделам страницы** — `TestsBlankPage.vue:24-32`
   (Список / Формы для меня / Конструктор / Статистика; последние две только
   при `canEditSection('tests')`).
2. **Компактный переключатель** — `variant="pill" size="xs" color="neutral"
   :content="false" activation-mode="manual"` в меню профиля.

### Прочее

| Компонент | Применение |
|-----------|------------|
| `UBadge` | ~61 вхождение — статусы, счётчики |
| `UAlert` | Постоянные предупреждения в контексте страницы |
| `UTooltip` | Пояснения к иконочным кнопкам |
| `UAvatar` | Аватары (`size="2xs"` в шапке) |
| `UProgress` | Прогресс прохождения курса |
| `UBreadcrumb` | Только в `AppHeader` |
| `UEditor` | Rich-text: новости, стена, документация (`:editable="false"`) |
| `USeparator`, `UScrollArea`, `UCarousel`, `UCalendar`, `UPopover`, `UDropdownMenu` | по месту |

---

## Кастомизация: три уровня

**[ПОДТВЕРЖДЕНО]**

### 1. Глобально — `vite.config.js`

Для правил, действующих во всём приложении: палитра, обнуление отступов
контейнера, ширина `main`, стиль `UPageHeader`, ширина сегментов `UInputDate`.

### 2. Точечно — проп `:ui`

Переопределение слотов конкретного экземпляра:

```html
<UDashboardSidebar :ui="{ root: '…', header: '…', body: '…', footer: '…' }">
```

Самый частый способ в проекте.

### 3. Классы Tailwind на компоненте

`class="…"` объединяется с базовыми стилями через `tailwind-merge`.
Для «пробивания» приоритета используется `!` (`!max-w-4xl`) — встречается в
ширинах `USlideover`.

### Глобальный CSS

`src/assets/tailwind.css` — только для того, что нельзя выразить утилитами:
палитры, `@theme`, скроллбары, коррекция весов Unbounded, `.scrollbar-hide`,
`.rounded-panel`.

`<style>` внутри SFC используется редко (`App.vue` — View Transitions,
`ProfilePage.vue` — обложка профиля).

---

## Правила создания нового компонента

1. Сначала убедиться, что подходящего нет ни в проекте, ни в Nuxt UI
   (см. [nuxt-ui.md](nuxt-ui.md) — инвентарь из 121 компонента, из них
   используется 52).
2. Размещение: общий → `src/components/`; для одного раздела → рядом со
   страницами этого раздела (как `src/pages/Courses/components/`).
3. `<script setup lang="ts">`, типизированные `defineProps`.
4. **Не** давать имя с префиксом `U` — он зарезервирован за Nuxt UI
   (удалённый 2026-09-25 `UContentSurround.vue` был ошибкой, а не образцом).
5. Только семантические цветовые токены, никаких `text-green-600`.
6. Собирать из компонентов Nuxt UI, а не из голой вёрстки.
7. Не импортировать вручную — автоимпорт подхватит.
8. Предусмотреть состояния загрузки / пустоты / ошибки.
9. Проверить обе темы и все брейкпоинты.
