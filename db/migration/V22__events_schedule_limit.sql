-- V22__events_schedule_limit.sql
-- Мероприятия (ADR-054): время начала и окончания, место и лимит мест отдельными полями.
-- Раньше время и место угадывались из текста описания («14:00», «место: …»), иначе —
-- «Время уточняется» / «Место уточняется». Лимит мест: NULL — без ограничения.
-- Повторно применима: deploy.sh прогоняет все V*.sql при каждом деплое.

ALTER TABLE public.events
  ADD COLUMN IF NOT EXISTS time_start       text,     -- 'HH:MM'
  ADD COLUMN IF NOT EXISTS time_end         text,     -- 'HH:MM'
  ADD COLUMN IF NOT EXISTS location         text,
  ADD COLUMN IF NOT EXISTS max_participants integer;  -- NULL — без ограничения

DO $$
BEGIN
  ALTER TABLE public.events
    ADD CONSTRAINT events_max_participants_chk CHECK (max_participants IS NULL OR max_participants > 0);
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

-- Список записавшихся по мероприятию — в порядке записи.
CREATE INDEX IF NOT EXISTS event_rsvps_event_created_idx ON public.event_rsvps (event_id, created_at);
