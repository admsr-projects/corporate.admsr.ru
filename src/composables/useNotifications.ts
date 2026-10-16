import { ref } from 'vue';
import { apiSessionFetch, getAuthUser } from './useAuthSession';
import { DEVBLOG_AUTHOR } from './useDevblog';

/**
 * Уведомления (колокольчик в шапке) — /api/notifications.php, таблица notifications (V17).
 * Виды: запись на вашей стене, поздравление с днём рождения, ответ на ваш
 * комментарий к новости (V18), напоминание накануне о личном событии или
 * мероприятии, куда вы записались (V19, создаёт сервер при запросе списка).
 *
 * Опроса по таймеру нет намеренно: каждый авторизованный запрос продлевает
 * last_activity, и опрос раз в минуту отменил бы выход по бездействию
 * (useSessionActivity). Обновляем при открытии колокольчика, при переходах
 * между страницами и возврате на вкладку — то есть при реальной активности.
 */
export type NotificationKind = 'wall_post' | 'birthday_greeting' | 'comment_reply' | 'event_reminder' | 'devblog';

export type NotificationReminder = {
  title: string;
  /** YYYY-MM-DD */
  date: string;
  timeStart: string;
  /** Окончание — для мероприятия: время, на которое записан (V23) */
  timeEnd: string;
  location: string;
  /** '#rrggbb' личного события или '' */
  color: string;
  calendarEntryId: number | null;
  eventId: number | null;
};

export type PortalNotification = {
  id: number;
  kind: NotificationKind;
  postId: number | null;
  /** comment_reply: куда вести — новость и комментарий */
  commentId: number | null;
  newsId: number | null;
  createdAt: string;
  read: boolean;
  /** null — системное (напоминание) */
  actor: { id: number; name: string; avatar_url: string } | null;
  excerpt: string;
  reminder: NotificationReminder | null;
};

export const NOTIFICATION_KIND_LABEL: Record<NotificationKind, string> = {
  wall_post: 'Запись на вашей стене',
  birthday_greeting: 'Поздравление с днём рождения',
  comment_reply: 'Ответ на ваш комментарий',
  devblog: 'Девблог: что нового на портале',
  event_reminder: 'Напоминание: событие завтра',
};

/** Кто / что в заголовке уведомления: автор, «Разработчики портала» для девблога или событие. */
export function notificationTitle(n: PortalNotification): string {
  if (n.actor) return n.actor.name;
  if (n.kind === 'devblog') return DEVBLOG_AUTHOR;
  return n.reminder?.title || 'Событие';
}

export const NOTIFICATION_KIND_ICON: Record<NotificationKind, string> = {
  wall_post: 'i-lucide-message-square',
  birthday_greeting: 'i-lucide-gift',
  comment_reply: 'i-lucide-message-circle-reply',
  devblog: 'i-lucide-notebook-pen',
  event_reminder: 'i-lucide-calendar-clock',
};

/** Не чаще раза в 30 с при переходах и возврате на вкладку; открытие колокольчика — всегда. */
const MIN_REFRESH_MS = 30_000;

const items = ref<PortalNotification[]>([]);
const unread = ref(0);
const loading = ref(false);
const loaded = ref(false);
const error = ref('');
let lastFetchAt = 0;
/** Чьи уведомления в памяти: после выхода и входа другим сотрудником чужие не показываем. */
let loadedFor = 0;
let inflight: Promise<void> | null = null;

function normalize(raw: any): PortalNotification {
  const kind: NotificationKind =
    raw?.kind === 'birthday_greeting' ||
    raw?.kind === 'comment_reply' ||
    raw?.kind === 'event_reminder' ||
    raw?.kind === 'devblog'
      ? raw.kind
      : 'wall_post';
  return {
    id: Number(raw?.id) || 0,
    kind,
    postId: raw?.postId != null ? Number(raw.postId) : null,
    commentId: raw?.commentId != null ? Number(raw.commentId) : null,
    newsId: raw?.newsId != null ? Number(raw.newsId) : null,
    createdAt: String(raw?.createdAt ?? ''),
    read: Boolean(raw?.read),
    // Девблог подписан «Разработчики портала», а не опубликовавшим администратором.
    actor: raw?.actor && kind !== 'devblog'
      ? {
          id: Number(raw.actor.id) || 0,
          name: String(raw.actor.name ?? 'Сотрудник'),
          avatar_url: String(raw.actor.avatar_url ?? ''),
        }
      : null,
    excerpt: String(raw?.excerpt ?? ''),
    reminder: raw?.reminder
      ? {
          title: String(raw.reminder.title ?? ''),
          date: String(raw.reminder.date ?? ''),
          timeStart: String(raw.reminder.timeStart ?? ''),
          timeEnd: String(raw.reminder.timeEnd ?? ''),
          location: String(raw.reminder.location ?? ''),
          color: String(raw.reminder.color ?? ''),
          calendarEntryId: raw.reminder.calendarEntryId != null ? Number(raw.reminder.calendarEntryId) : null,
          eventId: raw.reminder.eventId != null ? Number(raw.reminder.eventId) : null,
        }
      : null,
  };
}

export function useNotifications() {
  async function refresh(force = false) {
    const me = Number(getAuthUser()?.id) || 0;
    if (!me) return;
    if (me !== loadedFor) {
      items.value = [];
      unread.value = 0;
      loaded.value = false;
      loadedFor = me;
      force = true;
    }
    if (!force && Date.now() - lastFetchAt < MIN_REFRESH_MS) return;
    if (inflight) return inflight;
    lastFetchAt = Date.now();
    loading.value = true;
    inflight = (async () => {
      try {
        const json = await apiSessionFetch<{ unread?: number; items?: unknown[] }>('/api/notifications.php');
        if (!json?.success) throw new Error(json?.message || 'Не удалось загрузить уведомления');
        items.value = Array.isArray(json.data?.items) ? json.data!.items!.map(normalize) : [];
        unread.value = Number(json.data?.unread) || 0;
        loaded.value = true;
        error.value = '';
      } catch (e) {
        error.value = e instanceof Error ? e.message : 'Не удалось загрузить уведомления';
      } finally {
        loading.value = false;
        inflight = null;
      }
    })();
    return inflight;
  }

  /** Оптимистично; при отказе сервера откатывает отметку. */
  async function markRead(ids: number[]) {
    const targets = items.value.filter((n) => ids.includes(n.id) && !n.read);
    if (!targets.length) return;
    const prevUnread = unread.value;
    for (const n of targets) n.read = true;
    unread.value = Math.max(0, unread.value - targets.length);
    try {
      const json = await apiSessionFetch<{ unread?: number }>('/api/notifications.php', {
        method: 'POST',
        json: { action: 'read', ids: targets.map((n) => n.id) },
      });
      if (!json?.success) throw new Error(json?.message);
      unread.value = Number(json.data?.unread) || 0;
    } catch {
      for (const n of targets) n.read = false;
      unread.value = prevUnread;
    }
  }

  /** Оптимистично; при отказе сервера откатывает. Возвращает false, если не удалось. */
  async function markAllRead(): Promise<boolean> {
    const targets = items.value.filter((n) => !n.read);
    const prevUnread = unread.value;
    for (const n of targets) n.read = true;
    unread.value = 0;
    try {
      const json = await apiSessionFetch('/api/notifications.php', {
        method: 'POST',
        json: { action: 'read_all' },
      });
      if (!json?.success) throw new Error(json?.message);
      return true;
    } catch {
      for (const n of targets) n.read = false;
      unread.value = prevUnread;
      return false;
    }
  }

  return { items, unread, loading, loaded, error, refresh, markRead, markAllRead };
}
