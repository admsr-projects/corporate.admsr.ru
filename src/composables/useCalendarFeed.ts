import { computed, ref } from 'vue';
import { useBirthdayColleagues } from './useBirthdayColleagues';
import { useCoursesStore, type EnrollmentSummary } from './useCoursesStore';
import { apiSessionFetch, getAuthUser } from './useAuthSession';
import { useEventRsvp } from './useEventRsvp';
import { useBirthdayGreetings } from './useBirthdayGreetings';
import { eventTimeLabel, readEventSchedule } from './eventSchedule';

export type CalendarSource = 'event' | 'meeting' | 'birthday' | 'learning' | 'personal';

export type CalendarItem = {
  id: string;
  source: CalendarSource;
  /** YYYY-MM-DD */
  dateKey: string;
  title: string;
  timeStart?: string;
  timeEnd?: string;
  /** Short label for grid chips */
  timeLabel: string;
  location?: string;
  avatar?: string;
  role?: string;
  progress?: number;
  href?: string;
  /** Пусто — кнопки действия нет */
  actionLabel: string;
  isJoined?: boolean;
  /** День рождения: id учётной записи именинника (null — ФИО не сопоставилось) */
  userId?: number | null;
  /** День рождения: можно поздравить прямо сейчас (окно и «ещё не поздравил») */
  greetable?: boolean;
  /** Личное событие / встреча: цвет '#rrggbb', выбранный сотрудником (V19); нет — цвет по типу */
  color?: string;
};

export type LocalCalendarEntry = {
  /** id с сервера (число); в ленте календаря он превращается в строку `entry-<id>` */
  id: number | string;
  source: 'meeting' | 'personal';
  dateKey: string;
  title: string;
  timeStart?: string;
  timeEnd?: string;
  location?: string;
  /** '#rrggbb' или нет — цвет по типу */
  color?: string;
};

// Личные записи и встречи — на сервере (/api/calendar_entries.php, V16). Раньше лежали в
// localStorage одним списком на весь браузер — старые записи не переносятся (чьи они, неизвестно).
if (typeof window !== 'undefined') {
  try {
    window.localStorage.removeItem('portal-calendar-local:v1');
  } catch {
    /* хранилище недоступно */
  }
}

export const CALENDAR_SOURCE_META: Record<
  CalendarSource,
  { label: string; icon: string; barClass: string; softClass: string }
> = {
  event: {
    label: 'Мероприятия',
    icon: 'i-lucide-calendar',
    barClass: 'bg-emerald-500',
    softClass: 'border-emerald-500/30 bg-emerald-500/10',
  },
  meeting: {
    label: 'Встречи',
    icon: 'i-lucide-users',
    barClass: 'bg-info',
    softClass: 'border-info/30 bg-info/5',
  },
  birthday: {
    label: 'Дни рождения',
    icon: 'i-lucide-cake',
    barClass: 'bg-violet-500',
    softClass: 'border-violet-500/30 bg-violet-500/5',
  },
  learning: {
    label: 'Обучение',
    icon: 'i-lucide-graduation-cap',
    barClass: 'bg-warning',
    softClass: 'border-warning/30 bg-warning/5',
  },
  personal: {
    label: 'Личное',
    icon: 'i-lucide-circle',
    barClass: 'bg-muted',
    softClass: 'border-default bg-elevated/40',
  },
};

function pad2(n: number) {
  return String(n).padStart(2, '0');
}

export function toDateKey(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

export function parseDateKey(key: string): Date {
  const [y, m, d] = key.split('-').map(Number);
  return new Date(y, m - 1, d);
}

function extractTimeRange(text: string): { start?: string; end?: string; label: string } {
  const range = String(text ?? '').match(/(\d{1,2}:\d{2})\s*[-–—]\s*(\d{1,2}:\d{2})/);
  if (range) return { start: range[1], end: range[2], label: `${range[1]}-${range[2]}` };
  const single = String(text ?? '').match(/\b(\d{1,2}:\d{2})\b/);
  if (single) return { start: single[1], label: single[1] };
  return { label: '' };
}

function extractPlace(text: string): string | undefined {
  const m = String(text ?? '').match(/(?:адрес|место|зал|переговорн)[:\s]+([^.\n]+)/i);
  return m?.[1]?.trim() || undefined;
}

function formatHm(d: Date): string {
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

/**
 * Окраска события: цвет, выбранный сотрудником (V19, ADR-051), иначе — цвет по типу.
 * Одни и те же функции во всех местах, где показываются события календаря
 * (страница календаря, виджет на рабочем столе), — чтобы цвет совпадал везде.
 * bar — точка / полоса, soft — рамка и подложка карточки (30 % / 10 %, как у типов).
 */
export function calendarItemBarClass(item: CalendarItem): string {
  return item.color ? '' : CALENDAR_SOURCE_META[item.source].barClass;
}
export function calendarItemBarStyle(item: CalendarItem) {
  return item.color ? { backgroundColor: item.color } : undefined;
}
export function calendarItemSoftClass(item: CalendarItem): string {
  return item.color ? '' : CALENDAR_SOURCE_META[item.source].softClass;
}
export function calendarItemSoftStyle(item: CalendarItem) {
  return item.color ? { borderColor: `${item.color}4d`, backgroundColor: `${item.color}1a` } : undefined;
}

export function useCalendarFeed() {
  const {
    loading: birthdaysLoading,
    error: birthdaysError,
    ensureLoaded: ensureBirthdays,
    byMonthDay,
  } = useBirthdayColleagues();

  const courses = useCoursesStore();
  const { ensureLoaded: ensureRsvpLoaded, isJoined: isRsvpJoined } = useEventRsvp();
  const greetings = useBirthdayGreetings();

  const eventsLoading = ref(false);
  const eventsError = ref<string | null>(null);
  const rawEvents = ref<any[]>([]);
  const localEntries = ref<LocalCalendarEntry[]>([]);
  const learningLoading = ref(false);

  const loading = computed(
    () => birthdaysLoading.value || eventsLoading.value || learningLoading.value,
  );
  const error = computed(() => birthdaysError.value || eventsError.value);

  async function loadEvents() {
    eventsLoading.value = true;
    eventsError.value = null;
    try {
      const res = await fetch('/api/events.php', { cache: 'no-store' });
      if (!res.ok) throw new Error(`Не удалось загрузить мероприятия (${res.status})`);
      const json = await res.json();
      if (!json.success) throw new Error(json.message || 'Ошибка загрузки мероприятий');
      rawEvents.value = Array.isArray(json.data) ? json.data : [];
    } catch (e) {
      eventsError.value = e instanceof Error ? e.message : 'Ошибка загрузки мероприятий';
      rawEvents.value = [];
    } finally {
      eventsLoading.value = false;
    }
  }

  async function loadLearning() {
    learningLoading.value = true;
    try {
      await courses.loadMyCourses();
    } catch {
      // LMS may be unavailable — calendar still works without deadlines
    } finally {
      learningLoading.value = false;
    }
  }

  async function loadLocal() {
    if (!getAuthUser()?.id) {
      localEntries.value = [];
      return;
    }
    try {
      const json = await apiSessionFetch<{ entries?: LocalCalendarEntry[] }>('/api/calendar_entries.php');
      if (json?.success) {
        localEntries.value = Array.isArray(json.data?.entries) ? json.data!.entries! : [];
      }
    } catch {
      // календарь работает и без личных записей
    }
  }

  async function ensureLoaded() {
    ensureBirthdays();
    greetings.ensureLoaded();
    await Promise.all([loadEvents(), loadLearning(), loadLocal(), ensureRsvpLoaded()]);
  }

  /** Сохраняет на сервере; при отказе бросает Error с текстом для тоста. */
  async function addLocalEntry(entry: Omit<LocalCalendarEntry, 'id'>): Promise<LocalCalendarEntry> {
    const json = await apiSessionFetch<{ entry?: LocalCalendarEntry }>('/api/calendar_entries.php', {
      method: 'POST',
      json: {
        action: 'create',
        source: entry.source,
        dateKey: entry.dateKey,
        title: entry.title.trim(),
        timeStart: entry.timeStart,
        timeEnd: entry.timeEnd,
        location: entry.location,
        color: entry.color,
      },
    });
    const saved = json?.data?.entry;
    if (!json?.success || !saved) throw new Error(json?.message || 'Не удалось сохранить запись');
    localEntries.value = [...localEntries.value, saved];
    return saved;
  }

  /** Изменяет на сервере (название, дата, время, место, цвет); при отказе бросает Error. */
  async function updateLocalEntry(entry: LocalCalendarEntry): Promise<LocalCalendarEntry> {
    const json = await apiSessionFetch<{ entry?: LocalCalendarEntry }>('/api/calendar_entries.php', {
      method: 'POST',
      json: {
        action: 'update',
        id: Number(entry.id),
        source: entry.source,
        dateKey: entry.dateKey,
        title: entry.title.trim(),
        timeStart: entry.timeStart,
        timeEnd: entry.timeEnd,
        location: entry.location,
        color: entry.color,
      },
    });
    const saved = json?.data?.entry;
    if (!json?.success || !saved) throw new Error(json?.message || 'Не удалось сохранить запись');
    localEntries.value = localEntries.value.map((e) => (String(e.id) === String(saved.id) ? saved : e));
    return saved;
  }

  /** Удаляет на сервере; запись пропадает из списка только после подтверждения. */
  async function removeLocalEntry(id: string | number): Promise<void> {
    const json = await apiSessionFetch('/api/calendar_entries.php', {
      method: 'POST',
      json: { action: 'delete', id: Number(id) },
    });
    if (!json?.success) throw new Error(json?.message || 'Не удалось удалить запись');
    localEntries.value = localEntries.value.filter((e) => String(e.id) !== String(id));
  }

  const items = computed((): CalendarItem[] => {
    const out: CalendarItem[] = [];

    for (const ev of rawEvents.value) {
      const dateRaw = String(ev?.date ?? '').trim();
      const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(dateRaw);
      if (!m) continue;
      const dateKey = `${m[1]}-${m[2]}-${m[3]}`;
      const text = `${ev?.description ?? ''} ${ev?.title ?? ''}`;
      // Поля V22 (ADR-054); у старых мероприятий без них — из текста описания.
      const sched = readEventSchedule(ev);
      const time = sched.timeStart
        ? { start: sched.timeStart, end: sched.timeEnd || undefined, label: eventTimeLabel(sched) }
        : extractTimeRange(text);
      const id = String(ev.id);
      out.push({
        id: `event-${id}`,
        source: 'event',
        dateKey,
        title: String(ev.title ?? 'Мероприятие'),
        timeStart: time.start,
        timeEnd: time.end,
        timeLabel: time.label || time.start || '',
        location: sched.location || extractPlace(text),
        href: `/events/${id}`,
        actionLabel: 'Открыть',
        isJoined: isRsvpJoined(id),
      });
    }

    for (const entry of localEntries.value) {
      const label =
        entry.timeStart && entry.timeEnd
          ? `${entry.timeStart}-${entry.timeEnd}`
          : entry.timeStart || '';
      out.push({
        id: `entry-${entry.id}`,
        source: entry.source,
        dateKey: entry.dateKey,
        title: entry.title,
        timeStart: entry.timeStart,
        timeEnd: entry.timeEnd,
        timeLabel: label,
        location: entry.location,
        color: entry.color,
        // Открывать нечего — всё уже на карточке; правка и удаление — в меню «⋮».
        actionLabel: '',
      });
    }

    const yearHint = new Date().getFullYear();
    for (const [key, people] of Object.entries(byMonthDay.value)) {
      const [mm, dd] = key.split('-').map(Number);
      if (!mm || !dd) continue;
      // Birthdays are year-agnostic — emit for current + next calendar year span
      for (const year of [yearHint - 1, yearHint, yearHint + 1]) {
        const dateKey = `${year}-${pad2(mm)}-${pad2(dd)}`;
        const date = new Date(year, mm - 1, dd);
        for (const person of people) {
          const state = greetings.greetState(person.userId, date);
          out.push({
            id: `bday-${year}-${person.id}`,
            source: 'birthday',
            dateKey,
            title: person.fio,
            timeLabel: '',
            avatar: person.avatar,
            role: person.positionTitle || undefined,
            userId: person.userId,
            greetable: state === 'greet',
            href: person.userId ? `/profile/${person.userId}` : undefined,
            actionLabel:
              state === 'greet' ? 'Поздравить' : state === 'greeted' ? 'Вы поздравили' : person.userId ? 'Профиль' : '',
          });
        }
      }
    }

    for (const enr of courses.myEnrollments.value as EnrollmentSummary[]) {
      // Курс пройден — напоминание «Завершить» в календаре больше не нужно.
      if (enr.status === 'completed') continue;
      const raw = enr.deadlineAt;
      if (!raw) continue;
      const d = new Date(raw);
      if (Number.isNaN(d.getTime())) continue;
      const dateKey = toDateKey(d);
      const hm = formatHm(d);
      out.push({
        id: `learn-${enr.id}`,
        source: 'learning',
        dateKey,
        title: enr.courseTitle ? `Завершить: ${enr.courseTitle}` : 'Срок обучения',
        timeStart: hm,
        timeLabel: hm !== '00:00' ? `до ${hm}` : 'Срок',
        progress: Number(enr.progressPercent ?? 0),
        href: `/courses/${enr.id}`,
        actionLabel: 'Продолжить',
      });
    }

    out.sort((a, b) => {
      if (a.dateKey !== b.dateKey) return a.dateKey.localeCompare(b.dateKey);
      const ta = a.timeStart || '99:99';
      const tb = b.timeStart || '99:99';
      if (ta !== tb) return ta.localeCompare(tb);
      return a.title.localeCompare(b.title, 'ru');
    });

    return out;
  });

  return {
    loading,
    error,
    items,
    localEntries,
    ensureLoaded,
    reloadEvents: loadEvents,
    reloadLearning: loadLearning,
    addLocalEntry,
    updateLocalEntry,
    removeLocalEntry,
  };
}
