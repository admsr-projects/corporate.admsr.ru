/**
 * Время, место и лимит мест мероприятия (events.php, V22, ADR-054). Раньше этих полей не
 * было, и время с местом угадывались из текста описания; для старых мероприятий без
 * полей страницы по-прежнему пробуют описание.
 */

export type EventSchedule = {
  /** «13:00»; пусто — не указано */
  timeStart: string;
  timeEnd: string;
  location: string;
  /** null — без ограничения */
  maxParticipants: number | null;
  participantsCount: number;
};

export function readEventSchedule(raw: any): EventSchedule {
  const max = Number(raw?.max_participants);
  return {
    timeStart: String(raw?.time_start ?? ''),
    timeEnd: String(raw?.time_end ?? ''),
    location: String(raw?.location ?? ''),
    maxParticipants: Number.isFinite(max) && max > 0 ? max : null,
    participantsCount: Number(raw?.participants_count) || 0,
  };
}

/** «13:00–15:30», «13:00» или пусто. */
export function eventTimeLabel(s: Pick<EventSchedule, 'timeStart' | 'timeEnd'>): string {
  if (s.timeStart && s.timeEnd) return `${s.timeStart}–${s.timeEnd}`;
  return s.timeStart;
}

/** Мест не осталось: лимит есть и он заполнен. */
export function isEventFull(s: Pick<EventSchedule, 'maxParticipants' | 'participantsCount'>): boolean {
  return s.maxParticipants != null && s.participantsCount >= s.maxParticipants;
}

/** «Записалось 12 из 30» / «Записалось 12». */
export function eventSeatsLabel(s: Pick<EventSchedule, 'maxParticipants' | 'participantsCount'>): string {
  const n = s.participantsCount;
  return s.maxParticipants != null ? `Записалось ${n} из ${s.maxParticipants}` : `Записалось ${n}`;
}

/** Тело запроса events.php: пустое — не указано, лимит 0 / пусто — без ограничения. */
export function eventScheduleBody(s: Pick<EventSchedule, 'timeStart' | 'timeEnd' | 'location' | 'maxParticipants'>) {
  const max = Number(s.maxParticipants);
  return {
    time_start: s.timeStart || null,
    time_end: s.timeStart ? s.timeEnd || null : null,
    location: s.location.trim() || null,
    max_participants: Number.isFinite(max) && max > 0 ? Math.round(max) : null,
  };
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
