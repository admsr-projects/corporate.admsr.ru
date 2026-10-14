<script setup lang="ts">
/**
 * «Мероприятия» на рабочем столе (ADR-055): ближайшие предстоящие мероприятия из афиши —
 * в календарь мероприятие попадает только после записи, а здесь видны все новые.
 * Тот же вид, что у виджетов «Календарь» и «Дни рождения коллег».
 */
import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useEventRsvp } from '../../composables/useEventRsvp';
import { eventSeatsLabel, eventTimeLabel, isEventFull, readEventSchedule, type EventSchedule } from '../../composables/eventSchedule';
import { toDateKey } from '../../composables/useCalendarFeed';

const emit = defineEmits<{ visible: [value: boolean] }>();

const MAX_ITEMS = 5;
const monthNamesShort = ['янв', 'фев', 'мар', 'апр', 'май', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'] as const;
const weekdayShort = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб'] as const;

type HomeEvent = { id: number; title: string; dateKey: string; schedule: EventSchedule };

const router = useRouter();
const { ensureLoaded: ensureRsvpLoaded, isJoined } = useEventRsvp();

const loading = ref(true);
const error = ref('');
const events = ref<HomeEvent[]>([]);

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const today = toDateKey(new Date());
    const res = await fetch(`/api/events.php?date_from=${today}&order=date&dir=asc`, { cache: 'no-store' });
    const json = await res.json();
    if (!res.ok || !json?.success) throw new Error(json?.message || 'Не удалось загрузить мероприятия');
    const rows: any[] = Array.isArray(json.data) ? json.data : [];
    events.value = rows
      // «Архив» — запись закрыта, на рабочем столе такие не нужны.
      .filter((r) => !String(r?.badge ?? '').toLowerCase().includes('архив'))
      .map((r) => ({
        id: Number(r.id),
        title: String(r.title ?? 'Мероприятие'),
        dateKey: String(r.date ?? '').slice(0, 10),
        schedule: readEventSchedule(r),
      }))
      .filter((e) => e.id && /^\d{4}-\d{2}-\d{2}$/.test(e.dateKey) && e.dateKey >= today)
      .slice(0, MAX_ITEMS);
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Не удалось загрузить мероприятия';
    events.value = [];
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void load();
  void ensureRsvpLoaded();
});

function dateLabel(dateKey: string): string {
  const [y, m, d] = dateKey.split('-').map(Number);
  const date = new Date(y, m - 1, d);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const tomorrow = new Date(today);
  tomorrow.setDate(tomorrow.getDate() + 1);
  if (toDateKey(today) === dateKey) return 'Сегодня';
  if (toDateKey(tomorrow) === dateKey) return 'Завтра';
  return `${weekdayShort[date.getDay()]}, ${d} ${monthNamesShort[date.getMonth()]}`;
}

function metaLine(e: HomeEvent): string {
  return [dateLabel(e.dateKey), eventTimeLabel(e.schedule), e.schedule.location].filter(Boolean).join(' · ');
}

// Ошибка загрузки — тоже показываем, чтобы было видно, что мероприятия есть, но не загрузились.
const showWidget = computed(() => !loading.value && (events.value.length > 0 || !!error.value));
watch(showWidget, (v) => emit('visible', v), { immediate: true });
</script>

<template>
  <UCard
    v-if="showWidget"
    variant="soft"
    class="w-full rounded-panel"
    :ui="{
      root: 'rounded-panel bg-elevated ring-0 border-0 divide-y-0',
      header: 'px-4 py-4 sm:px-4',
      body: 'flex flex-col gap-1.5 px-4 pb-4 pt-0 sm:px-4 sm:pb-4 sm:pt-0',
    }"
  >
    <template #header>
      <div class="flex items-center justify-between gap-1">
        <div class="flex items-center gap-1 min-w-0">
          <h2 class="text-lg font-bold leading-7 text-highlighted truncate">Мероприятия</h2>
          <UTooltip text="Ближайшие мероприятия. Запишитесь — и мероприятие появится в вашем календаре">
            <UButton
              type="button"
              color="neutral"
              variant="ghost"
              size="xs"
              icon="i-lucide-info"
              square
              aria-label="О мероприятиях"
            />
          </UTooltip>
        </div>
        <UTooltip text="Все мероприятия">
          <UButton
            to="/events"
            color="neutral"
            variant="ghost"
            size="xs"
            icon="i-lucide-arrow-up-right"
            square
            aria-label="Все мероприятия"
          />
        </UTooltip>
      </div>
    </template>

    <p v-if="error" class="text-sm text-error">{{ error }}</p>
    <button
      v-for="e in events"
      :key="e.id"
      type="button"
      class="flex w-full items-start gap-2 rounded-lg bg-default/40 px-2.5 py-2 text-left transition-colors hover:bg-elevated/70"
      @click="router.push(`/events/${e.id}`)"
    >
      <UIcon name="i-lucide-calendar-heart" class="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <p class="text-sm font-medium text-highlighted line-clamp-2">{{ e.title }}</p>
        <p class="text-xs text-muted truncate">{{ metaLine(e) }}</p>
        <p class="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs">
          <span v-if="isJoined(e.id)" class="inline-flex items-center gap-1 text-primary">
            <UIcon name="i-lucide-check" class="size-3" aria-hidden="true" />
            Вы записаны
          </span>
          <span v-else-if="isEventFull(e.schedule)" class="text-warning">Мест нет</span>
          <span class="text-dimmed">{{ eventSeatsLabel(e.schedule) }}</span>
        </p>
      </div>
    </button>
  </UCard>
</template>
