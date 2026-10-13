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
