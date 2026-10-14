<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import {
  CALENDAR_SOURCE_META,
  calendarItemBarClass,
  calendarItemBarStyle,
  toDateKey,
  useCalendarFeed,
  type CalendarItem,
  type CalendarSource,
} from '../../composables/useCalendarFeed';

const emit = defineEmits<{ visible: [value: boolean] }>();

const router = useRouter();
const { loading, error, items, ensureLoaded } = useCalendarFeed();

onMounted(() => {
  ensureLoaded();
});

const monthNamesShort = [
  'янв', 'фев', 'мар', 'апр', 'май', 'июн',
  'июл', 'авг', 'сен', 'окт', 'ноя', 'дек',
] as const;

const weekdayShort = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб'] as const;

type HomeCalRow = {
  key: string;
  dateLabel: string;
  items: CalendarItem[];
};

const upcomingRows = computed((): HomeCalRow[] => {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const horizon = new Date(today);
  horizon.setDate(horizon.getDate() + 14);

  const byDate = new Map<string, CalendarItem[]>();
  for (const item of items.value) {
    if (item.source === 'birthday') continue;
    const [y, m, d] = item.dateKey.split('-').map(Number);
    if (!y || !m || !d) continue;
    const date = new Date(y, m - 1, d);
    if (date < today || date > horizon) continue;
    const list = byDate.get(item.dateKey) ?? [];
    list.push(item);
    byDate.set(item.dateKey, list);
  }

  const keys = [...byDate.keys()].sort();
  const rows: HomeCalRow[] = [];
  let shown = 0;
  for (const key of keys) {
    const dayItems = byDate.get(key) ?? [];
    if (!dayItems.length) continue;
    const [y, m, d] = key.split('-').map(Number);
    const date = new Date(y, m - 1, d);
    const isToday = toDateKey(today) === key;
    const isTomorrow = (() => {
      const t = new Date(today);
      t.setDate(t.getDate() + 1);
      return toDateKey(t) === key;
    })();
    let dateLabel = `${weekdayShort[date.getDay()]}, ${d} ${monthNamesShort[date.getMonth()]}`;
    if (isToday) dateLabel = 'Сегодня';
    else if (isTomorrow) dateLabel = 'Завтра';

    rows.push({ key, dateLabel, items: dayItems.slice(0, 4) });
    shown += dayItems.length;
    if (rows.length >= 4 || shown >= 8) break;
  }
  return rows;
});

const showWidget = computed(() => !loading.value && upcomingRows.value.length > 0);

watch(showWidget, (v) => emit('visible', v), { immediate: true });

function sourceMeta(source: CalendarSource) {
  return CALENDAR_SOURCE_META[source];
}

function openItem(item: CalendarItem) {
  if (item.href) {
    void router.push(item.href);
    return;
  }
  void router.push({ name: 'calendar' });
}
</script>

<template>
  <UCard
    v-if="showWidget"
    variant="soft"
    class="w-full rounded-panel"
    :ui="{
      root: 'rounded-panel bg-elevated ring-0 border-0 divide-y-0',
      header: 'px-4 py-4 sm:px-4',
      body: 'flex flex-col gap-3 px-4 pb-4 pt-0 sm:px-4 sm:pb-4 sm:pt-0',
    }"
  >
    <template #header>
      <div class="flex items-center justify-between gap-1">
        <div class="flex items-center gap-1 min-w-0">
          <h2 class="text-lg font-bold leading-7 text-highlighted truncate">
            Календарь
          </h2>
          <UTooltip text="Ближайшие встречи, мероприятия, на которые вы записались, обучение и личные события">
            <UButton
              type="button"
              color="neutral"
              variant="ghost"
              size="xs"
              icon="i-lucide-info"
              square
              aria-label="О календаре"
            />
          </UTooltip>
        </div>
        <UTooltip text="Открыть календарь">
          <UButton
            to="/calendar"
            color="neutral"
            variant="ghost"
            size="xs"
            icon="i-lucide-arrow-up-right"
            square
            aria-label="Открыть календарь" />
        </UTooltip>
      </div>
    </template>

    <div v-if="loading" class="flex flex-col gap-2">
      <USkeleton v-for="n in 3" :key="n" class="h-14 w-full rounded-lg" />
    </div>
    <p v-else-if="error" class="text-sm text-error">{{ error }}</p>
    <div
      v-for="row in upcomingRows"
      :key="row.key"
      class="flex flex-col gap-1.5"
    >
      <p class="text-xs font-medium text-muted">{{ row.dateLabel }}</p>
      <button
        v-for="item in row.items"
        :key="item.id"
        type="button"
        class="flex w-full items-start gap-2 rounded-lg bg-default/40 px-2.5 py-2 text-left transition-colors hover:bg-elevated/70"
        @click="openItem(item)"
      >
        <span
          class="mt-1.5 size-2 shrink-0 rounded-full"
          :class="calendarItemBarClass(item)"
          :style="calendarItemBarStyle(item)"
        />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-highlighted line-clamp-2">
            {{ item.title }}
          </p>
          <p class="text-xs text-muted truncate">
            {{ sourceMeta(item.source).label }}
            <template v-if="item.timeLabel"> · {{ item.timeLabel }}</template>
            <template v-if="item.location"> · {{ item.location }}</template>
          </p>
        </div>
      </button>
    </div>
  </UCard>
</template>
