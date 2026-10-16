/**
 * Время, место и лимит мест мероприятия (events.php). Место — одно на мероприятие (V22,
 * ADR-054); времён может быть несколько, у каждого свой лимит (V23, ADR-056) —
 * сотрудник записывается на одно из них. У старых мероприятий без полей страницы
 * по-прежнему ищут время и место в тексте описания.
 */

export type EventSlot = {
  /** 0 — ещё не сохранено */
  id: number;
  /** «13:00»; пусто — время уточняется */
  timeStart: string;
  timeEnd: string;
  /** null — без ограничения */
  maxParticipants: number | null;
  participantsCount: number;
};

export type EventSchedule = {
  location: string;
  /** Времена в порядке показа; всегда хотя бы одно */
  slots: EventSlot[];
  /** Записалось на все времена */
  participantsCount: number;
  /** Первое время — для мест, где показывается одно время */
  timeStart: string;
  timeEnd: string;
  /** Всего мест, если лимит есть у каждого времени; иначе null */
  maxParticipants: number | null;
};

function readLimit(v: unknown): number | null {
  const n = Number(v);
  return v != null && Number.isFinite(n) && n > 0 ? n : null;
}

export function readEventSchedule(raw: any): EventSchedule {
  let slots: EventSlot[] = Array.isArray(raw?.slots)
    ? raw.slots.map((s: any) => ({
        id: Number(s?.id) || 0,
        timeStart: String(s?.time_start ?? ''),
        timeEnd: String(s?.time_end ?? ''),
        maxParticipants: readLimit(s?.max_participants),
        participantsCount: Number(s?.participants_count) || 0,
      }))
    : [];
  if (!slots.length) {
    // Ответ без времён (сервер до V23) — одно время из полей V22.
    slots = [{
      id: 0,
      timeStart: String(raw?.time_start ?? ''),
      timeEnd: String(raw?.time_end ?? ''),
      maxParticipants: readLimit(raw?.max_participants),
      participantsCount: Number(raw?.participants_count) || 0,
    }];
  }
  const first = slots[0]!;
  const allLimited = slots.every((s) => s.maxParticipants != null);
  return {
    location: String(raw?.location ?? ''),
    slots,
    participantsCount: Number(raw?.participants_count) || slots.reduce((n, s) => n + s.participantsCount, 0),
    timeStart: first.timeStart,
    timeEnd: first.timeEnd,
    maxParticipants: allLimited ? slots.reduce((n, s) => n + (s.maxParticipants ?? 0), 0) : null,
  };
}

/** «13:00–15:30», «13:00» или пусто. */
export function slotTimeLabel(s: Pick<EventSlot, 'timeStart' | 'timeEnd'>): string {
  if (s.timeStart && s.timeEnd) return `${s.timeStart}–${s.timeEnd}`;
  return s.timeStart;
}

/** Все времена мероприятия через запятую: «11:00–13:00, 12:00–14:00»; пусто — не указано. */
export function eventTimeLabel(s: Pick<EventSchedule, 'slots'>): string {
  return s.slots.map(slotTimeLabel).filter(Boolean).join(', ');
}

/** На это время мест не осталось. */
export function isSlotFull(s: Pick<EventSlot, 'maxParticipants' | 'participantsCount'>): boolean {
  return s.maxParticipants != null && s.participantsCount >= s.maxParticipants;
}

/** Мест не осталось ни на одно время. */
export function isEventFull(s: Pick<EventSchedule, 'slots'>): boolean {
  return s.slots.length > 0 && s.slots.every(isSlotFull);
}

/** «Записалось 12 из 30» / «Записалось 12» — на все времена. */
export function eventSeatsLabel(s: Pick<EventSchedule, 'maxParticipants' | 'participantsCount'>): string {
  const n = s.participantsCount;
  return s.maxParticipants != null ? `Записалось ${n} из ${s.maxParticipants}` : `Записалось ${n}`;
}

/** «12 из 30» / «записалось 12» — на одно время. */
export function slotSeatsLabel(s: Pick<EventSlot, 'maxParticipants' | 'participantsCount'>): string {
  return s.maxParticipants != null ? `${s.participantsCount} из ${s.maxParticipants}` : `записалось ${s.participantsCount}`;
}

/** Время в форме создания и правки. */
export type EventSlotDraft = {
  id: number;
  timeStart: string;
  timeEnd: string;
  maxParticipants: number | null;
  /** Сколько уже записано — для предупреждения при удалении времени */
  participantsCount: number;
};

export function emptySlotDraft(timeStart = ''): EventSlotDraft {
  return { id: 0, timeStart, timeEnd: '', maxParticipants: null, participantsCount: 0 };
}

export function slotDraftsOf(s: EventSchedule): EventSlotDraft[] {
  return s.slots.map((x) => ({ ...x }));
}

/** Тело запроса events.php: место и времена. Пусто — не указано, лимит 0 / пусто — без ограничения. */
export function eventScheduleBody(s: { location: string; slots: EventSlotDraft[] }) {
  return {
    location: s.location.trim() || null,
    slots: s.slots.map((x) => {
      const max = Number(x.maxParticipants);
      return {
        id: x.id || undefined,
        time_start: x.timeStart || null,
        time_end: x.timeStart ? x.timeEnd || null : null,
        max_participants: Number.isFinite(max) && max > 0 ? Math.round(max) : null,
      };
    }),
  };
}

/** Окончание раньше начала у какого-нибудь времени — номер времени (с 1) или 0. */
export function badSlotOrder(slots: EventSlotDraft[]): number {
  const i = slots.findIndex((x) => x.timeStart && x.timeEnd && x.timeEnd < x.timeStart);
  return i + 1;
}

/**
 * Описание мероприятия — HTML из редактора, как у новостей (ADR-055). Старые описания —
 * обычный текст с переносами строк; их показываем как текст.
 */
export function isHtmlDescription(text: string): boolean {
  return /<\/?[a-z][^>]*>/i.test(text);
}

/** Текст без разметки — для превью, поиска времени и места в старых описаниях. */
export function eventDescriptionText(text: string): string {
  const s = String(text ?? '');
  if (!isHtmlDescription(s)) return s;
  const doc = new DOMParser().parseFromString(s, 'text/html');
  doc.querySelectorAll('p, li, h1, h2, h3, h4, h5, h6, blockquote, br').forEach((el) => el.append(' '));
  return (doc.body.textContent ?? '').replace(/\s+/g, ' ').trim();
}

/** Старое текстовое описание → абзацы для редактора, чтобы переносы строк не пропали. */
export function eventDescriptionForEditor(text: string): string {
  const s = String(text ?? '');
  if (!s.trim() || isHtmlDescription(s)) return s;
  const esc = (t: string) => t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  return s
    .split(/\r?\n/)
    .map((line) => `<p>${esc(line)}</p>`)
    .join('');
}

/** Для сохранения: пустой редактор («<p></p>») — null. */
export function eventDescriptionBody(html: string): string | null {
  return eventDescriptionText(html).trim() || /<img[\s>]/i.test(html) ? html.trim() : null;
}
