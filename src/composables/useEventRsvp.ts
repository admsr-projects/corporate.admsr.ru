import { ref } from 'vue';
import { apiSessionFetch, getAuthUser } from './useAuthSession';

/**
 * Запись на мероприятия — на сервере (/api/event_rsvp.php, таблица event_rsvps, V16).
 * Раньше хранилась в localStorage (`events-rsvp:v1`) одним общим словарём на браузер, поэтому
 * старые отметки не переносятся: нельзя понять, чьи они (на одном компьютере бывает несколько
 * сотрудников), и их не видел никто, кроме этого браузера.
 * Состояние общее для всех страниц: список мероприятий, карточка и календарь видят одно и то же.
 */
if (typeof window !== 'undefined') {
  try {
    window.localStorage.removeItem('events-rsvp:v1');
  } catch {
    /* хранилище недоступно */
  }
}

const joinedIds = ref<Set<number>>(new Set());
const loading = ref(false);
/** Для какого пользователя загружен список: после смены учётной записи грузим заново. */
let loadedFor: number | null = null;
let loadPromise: Promise<void> | null = null;

export function useEventRsvp() {
  async function ensureLoaded(force = false): Promise<void> {
    const uid = getAuthUser()?.id ?? null;
    if (!uid) {
      joinedIds.value = new Set();
      loadedFor = null;
      return;
    }
    if (!force && loadedFor === uid) return;
    if (loadPromise) return loadPromise;
    loading.value = true;
    loadPromise = (async () => {
      const json = await apiSessionFetch<{ eventIds?: number[] }>('/api/event_rsvp.php');
      if (json?.success) {
        const ids = Array.isArray(json.data?.eventIds) ? json.data!.eventIds! : [];
        joinedIds.value = new Set(ids.map(Number));
        loadedFor = uid;
      }
    })()
      .catch(() => {
        /* сеть недоступна — кнопки покажут «Записаться», повторим при следующем открытии */
      })
      .finally(() => {
        loading.value = false;
        loadPromise = null;
      });
    return loadPromise;
  }

  function isJoined(eventId: number | string | null | undefined): boolean {
    if (eventId == null) return false;
    return joinedIds.value.has(Number(eventId));
  }

  /**
   * Записаться / отменить запись. Список обновляется сразу и откатывается, если сервер
   * не подтвердил (проверяем и res.ok, и success — apiSessionFetch отдаёт оба случая как success=false).
   */
  async function setJoined(
    eventId: number,
    joined: boolean,
  ): Promise<{ ok: boolean; message?: string; participantsCount?: number }> {
    const id = Number(eventId);
    const next = new Set(joinedIds.value);
    if (joined) next.add(id);
    else next.delete(id);
    joinedIds.value = next;
    try {
      const json = await apiSessionFetch<{ participantsCount?: number }>('/api/event_rsvp.php', {
        method: 'POST',
        json: { eventId: id, joined },
      });
      // 409 — мест больше нет (лимит, V22): сообщение сервера уходит в тост.
      if (!json?.success) throw new Error(json?.message || 'Не удалось сохранить запись');
      const count = Number(json.data?.participantsCount);
      return { ok: true, participantsCount: Number.isFinite(count) ? count : undefined };
    } catch (e) {
      const back = new Set(joinedIds.value);
      if (joined) back.delete(id);
      else back.add(id);
      joinedIds.value = back;
      return { ok: false, message: e instanceof Error ? e.message : 'Не удалось сохранить запись' };
    }
  }

  return { joinedIds, loading, ensureLoaded, isJoined, setJoined };
}
