import { ref } from 'vue';
import { apiSessionFetch, getAuthUser } from './useAuthSession';

/**
 * Запись на мероприятия — на сервере (/api/event_rsvp.php, таблица event_rsvps, V16).
 * Запись — на конкретное время мероприятия (V23, ADR-056): у сотрудника одно время на
 * мероприятие, выбор другого — перезапись.
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

/** id мероприятия → id времени, на которое записан (0 — время неизвестно, запись до V23). */
const joined = ref<Map<number, number>>(new Map());
const loading = ref(false);
/** Для какого пользователя загружен список: после смены учётной записи грузим заново. */
let loadedFor: number | null = null;
let loadPromise: Promise<void> | null = null;

export type RsvpResult = {
  ok: boolean;
  message?: string;
  /** Записалось на все времена после изменения */
  participantsCount?: number;
  /** Записалось по каждому времени после изменения */
  slots?: { id: number; participantsCount: number }[];
};

export function useEventRsvp() {
  async function ensureLoaded(force = false): Promise<void> {
    const uid = getAuthUser()?.id ?? null;
    if (!uid) {
      joined.value = new Map();
      loadedFor = null;
      return;
    }
    if (!force && loadedFor === uid) return;
    if (loadPromise) return loadPromise;
    loading.value = true;
    loadPromise = (async () => {
      const json = await apiSessionFetch<{ eventIds?: number[]; items?: { eventId: number; slotId: number | null }[] }>(
        '/api/event_rsvp.php',
      );
      if (json?.success) {
        const next = new Map<number, number>();
        for (const id of json.data?.eventIds ?? []) next.set(Number(id), 0);
        for (const it of json.data?.items ?? []) next.set(Number(it.eventId), Number(it.slotId) || 0);
        joined.value = next;
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
    return joined.value.has(Number(eventId));
  }

  /** На какое время записан (id времени) или null — не записан; 0 — время неизвестно. */
  function joinedSlot(eventId: number | string | null | undefined): number | null {
    if (eventId == null) return null;
    const v = joined.value.get(Number(eventId));
    return v === undefined ? null : v;
  }

  /**
   * Записаться (на время slotId; не передано — первое) / перезаписаться / отменить запись.
   * Состояние обновляется сразу и откатывается, если сервер не подтвердил (проверяем и
   * res.ok, и success — apiSessionFetch отдаёт оба случая как success=false).
   */
  async function setJoined(eventId: number, join: boolean, slotId?: number): Promise<RsvpResult> {
    const id = Number(eventId);
    const prev = joined.value;
    const next = new Map(prev);
    if (join) next.set(id, slotId ?? 0);
    else next.delete(id);
    joined.value = next;
    try {
      const json = await apiSessionFetch<{
        participantsCount?: number;
        slotId?: number;
        slots?: { id: number; participantsCount: number }[];
      }>('/api/event_rsvp.php', {
        method: 'POST',
        json: { eventId: id, joined: join, slotId: slotId || undefined },
      });
      // 409 — на это время мест больше нет (лимит): сообщение сервера уходит в тост.
      if (!json?.success) throw new Error(json?.message || 'Не удалось сохранить запись');
      if (join && json.data?.slotId) {
        const fixed = new Map(joined.value);
        fixed.set(id, Number(json.data.slotId));
        joined.value = fixed;
      }
      const count = Number(json.data?.participantsCount);
      return {
        ok: true,
        participantsCount: Number.isFinite(count) ? count : undefined,
        slots: Array.isArray(json.data?.slots)
          ? json.data!.slots!.map((s) => ({ id: Number(s.id), participantsCount: Number(s.participantsCount) || 0 }))
          : undefined,
      };
    } catch (e) {
      joined.value = prev;
      return { ok: false, message: e instanceof Error ? e.message : 'Не удалось сохранить запись' };
    }
  }

  return { loading, ensureLoaded, isJoined, joinedSlot, setJoined };
}

/** Обновить счётчики расписания по ответу записи. */
export function applyRsvpCounts<T extends { participantsCount: number; slots: { id: number; participantsCount: number }[] }>(
  schedule: T,
  res: RsvpResult,
): T {
  if (res.participantsCount == null) return schedule;
  const by = new Map((res.slots ?? []).map((s) => [s.id, s.participantsCount]));
  return {
    ...schedule,
    participantsCount: res.participantsCount,
    slots: schedule.slots.map((s) => (by.has(s.id) ? { ...s, participantsCount: by.get(s.id)! } : s)),
  };
}
