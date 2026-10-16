-- V23__event_slots.sql
-- Несколько времён у мероприятия (ADR-056): этапы/потоки в один день, у каждого свой лимит мест.
-- Сотрудник записывается на одно время мероприятия (PK event_rsvps (event_id, user_id) остаётся).
-- events.time_start / time_end / max_participants (V22) больше не используются — перенесены в первое время.
-- Повторно применима: deploy.sh прогоняет все V*.sql при каждом деплое.

CREATE TABLE IF NOT EXISTS public.event_slots (
  id               bigserial   PRIMARY KEY,
  event_id         bigint      NOT NULL REFERENCES public.events (id) ON DELETE CASCADE,
  position         integer     NOT NULL DEFAULT 0,
  time_start       text,                   -- 'HH:MM'; NULL — время уточняется
  time_end         text,                   -- 'HH:MM'
  max_participants integer,                -- NULL — без ограничения
  created_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT event_slots_max_participants_chk CHECK (max_participants IS NULL OR max_participants > 0)
);

CREATE INDEX IF NOT EXISTS event_slots_event_idx ON public.event_slots (event_id, position, id);

-- Время, на которое записан сотрудник. Удалили время — записи на него тоже.
ALTER TABLE public.event_rsvps
  ADD COLUMN IF NOT EXISTS slot_id bigint REFERENCES public.event_slots (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS event_rsvps_slot_idx ON public.event_rsvps (slot_id);

-- У каждого мероприятия — хотя бы одно время: перенос из V22 (время, лимит).
INSERT INTO public.event_slots (event_id, position, time_start, time_end, max_participants)
SELECT e.id, 0, e.time_start, e.time_end, e.max_participants
FROM public.events e
WHERE NOT EXISTS (SELECT 1 FROM public.event_slots s WHERE s.event_id = e.id);

-- Записи без времени (сделанные до V23 или старым кодом) — на первое время мероприятия.
UPDATE public.event_rsvps r
SET slot_id = (
  SELECT s.id FROM public.event_slots s
  WHERE s.event_id = r.event_id
  ORDER BY s.position, s.id
  LIMIT 1)
WHERE r.slot_id IS NULL;
