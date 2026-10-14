<script setup lang="ts">
import { computed, ref, watch, reactive, onMounted, onUnmounted } from 'vue';
import { useRoute } from 'vue-router';
import type { BlogPostProps, TabsItem } from '@nuxt/ui';
import { useSectionAccess } from '../../composables/useSectionAccess';
import { apiSessionFetch } from '../../composables/useAuthSession';
import { formatDateRuLong } from '../../utils/date';
import { useGalleryData } from '../../composables/useGalleryData';
import { slideoverPopoverContent, slideoverSelectContent } from '../../composables/slideoverFieldUi';
import { useCursorFeed } from '../../composables/useCursorFeed';
import { useFeedSentinel } from '../../composables/useFeedSentinel';
import { useAppToast } from '../../composables/useAppToast';
import { useEventRsvp } from '../../composables/useEventRsvp';
import {
  eventDescriptionBody,
  eventDescriptionText,
  eventScheduleBody,
  eventSeatsLabel,
  eventTimeLabel,
  isEventFull,
  readEventSchedule,
  type EventSchedule,
} from '../../composables/eventSchedule';
import EventScheduleFields from './components/EventScheduleFields.vue';
import EventDescriptionEditor from './components/EventDescriptionEditor.vue';

type EventPost = BlogPostProps & {
  id?: number;
  badge?: string;
  rawDate?: string;
  description?: string;
  schedule: EventSchedule;
};

type EventCardModel = EventPost & {
  day: string;
  monthShort: string;
  timeLabel: string;
  placeLabel: string;
  isPast: boolean;
  isArchived: boolean;
  isJoined: boolean;
  /** Лимит мест заполнен (V22) */
  isFull: boolean;
  seatsLabel: string;
};

const EVENTS_PAGE_LIMIT = 24;
const MONTH_SHORT = ['ЯНВ', 'ФЕВ', 'МАР', 'АПР', 'МАЙ', 'ИЮН', 'ИЮЛ', 'АВГ', 'СЕН', 'ОКТ', 'НОЯ', 'ДЕК'] as const;

const route = useRoute();
const { toast } = useAppToast();
const isKiosk = computed(() => route.matched?.some((r) => r.meta?.kiosk));
const { canEditSection, ensureLoaded: ensureSectionAccess } = useSectionAccess();
ensureSectionAccess();
const { albums, ensureLoaded: ensureAlbumsLoaded } = useGalleryData();
ensureAlbumsLoaded();

const albumSelectItems = computed(() => [
  { label: 'Без альбома', value: 'none' },
  ...albums.value.map((a) => ({ label: a.title, value: String(a.id) })),
]);

// ─── Filters ─────────────────────────────────────────────────────────────────
const searchQuery = ref('');
const searchForApi = ref('');
let searchTimer: ReturnType<typeof setTimeout> | null = null;
watch(searchQuery, (q) => {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    searchForApi.value = q.trim();
  }, 300);
});

const badgeFilter = ref<'_all' | 'Новое' | 'Архив'>('_all');
const datePeriod = ref<'_all' | 'week' | 'month'>('_all');
const eventsTab = ref<'all' | 'my'>('all');
const { ensureLoaded: ensureRsvpLoaded, isJoined, setJoined } = useEventRsvp();

const datePeriodOptions = [
  { value: '_all', label: 'Любая дата' },
  { value: 'week', label: 'Ближайшая неделя' },
  { value: 'month', label: 'Ближайший месяц' },
];

const typeFilterOptions = [
  { value: '_all', label: 'Все типы' },
  { value: 'Новое', label: 'Новое' },
  { value: 'Архив', label: 'Архив' },
];

const createBadgeOptions = [
  { value: 'Новое', label: 'Новое' },
  { value: 'Архив', label: 'Архив' },
];

function isArchivedBadge(value: unknown) {
  return String(value ?? '').trim().toLowerCase().includes('архив');
}

function firstSentence(text: string): string {
  if (!text) return text;
  return text.match(/^[^.!?]*[.!?]/)?.[0].trim() ?? text;
}

function parseEventDate(raw?: string): Date | null {
  const s = String(raw ?? '').trim();
  if (!s) return null;
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(s);
  if (m) {
    const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
    return Number.isNaN(d.getTime()) ? null : d;
  }
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? null : d;
}

function startOfToday(): Date {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return d;
}

function extractTimeLabel(text: string): string {
  const m = String(text ?? '').match(/(\d{1,2}:\d{2})\s*[-–—]\s*(\d{1,2}:\d{2})/);
  if (m) return `${m[1]}-${m[2]}`;
  const single = String(text ?? '').match(/\b(\d{1,2}:\d{2})\b/);
  return single ? single[1] : 'Время уточняется';
}

function extractPlaceLabel(text: string): string {
  const m = String(text ?? '').match(/(?:адрес|место|зал)[:\s]+([^.\n]+)/i);
  if (m?.[1]) return m[1].trim();
  return 'Место уточняется';
}

function mapEvent(raw: any): EventPost {
  const rawDate = String(raw?.date ?? '').trim();
  // Описание — HTML из редактора (ADR-055); для карточки нужен текст.
  const fullDescription = eventDescriptionText(String(raw?.description ?? ''));
  return {
    id: raw.id,
    title: raw.title,
    description: firstSentence(fullDescription),
    badge: raw.badge ?? undefined,
    rawDate,
    date: formatDateRuLong(rawDate) || rawDate,
    to: `/events/${raw.id}`,
    image: raw.image ? { src: raw.image, alt: raw.title } : undefined,
    schedule: readEventSchedule(raw),
  };
}

async function toggleJoin(event: EventPost, e?: Event) {
  e?.preventDefault();
  e?.stopPropagation();
  const id = event.id;
  if (!id) return;
  if (isArchivedBadge(event.badge)) return;
  const wasJoined = isJoined(id);
  if (!wasJoined && isEventFull(event.schedule)) return;
  const res = await setJoined(id, !wasJoined);
  if (!res.ok) {
    toast.add({
      title: 'Не удалось сохранить запись',
      description: res.message,
      color: 'error',
      icon: 'i-lucide-alert-circle',
    });
    return;
  }
  // Число записавшихся — с сервера: карточки показывают «Записалось N из M».
  const post = posts.value.find((p) => p.id === id);
  if (post && res.participantsCount != null) {
    post.schedule = { ...post.schedule, participantsCount: res.participantsCount };
  }
  toast.add({
    title: wasJoined ? 'Запись отменена' : 'Вы записались на мероприятие',
    color: 'success',
    icon: 'i-lucide-circle-check',
  });
}

function toCardModel(event: EventPost): EventCardModel {
  const d = parseEventDate(event.rawDate);
  const text = `${event.description ?? ''} ${event.title ?? ''}`;
  const archived = isArchivedBadge(event.badge);
  const past = archived || (!!d && d < startOfToday());
  return {
    ...event,
    day: d ? String(d.getDate()).padStart(2, '0') : '—',
    monthShort: d ? MONTH_SHORT[d.getMonth()]! : '—',
    // Поля V22; у старых мероприятий без них — как раньше, из текста описания.
    timeLabel: eventTimeLabel(event.schedule) || extractTimeLabel(text),
    placeLabel: event.schedule.location || extractPlaceLabel(text),
    isPast: past,
    isArchived: archived,
    isJoined: isJoined(event.id),
    isFull: isEventFull(event.schedule),
    seatsLabel: eventSeatsLabel(event.schedule),
  };
}

const {
  items: posts,
  loading: feedLoading,
  initialLoading,
  error: fetchError,
  loadInitial,
  loadMore,
  refresh,
  sentinelEnabled,
} = useCursorFeed<EventPost>({
  buildUrl: (cursor) => {
    const params = new URLSearchParams();
    params.set('limit', String(EVENTS_PAGE_LIMIT));
    if (cursor) params.set('cursor', cursor);
    if (searchForApi.value) params.set('search', searchForApi.value);
    if (badgeFilter.value !== '_all') params.set('badge', badgeFilter.value);
    return `/api/events.php?${params.toString()}`;
  },
  mapItem: (raw) => mapEvent(raw),
  getId: (item) => String(item.id ?? item.title ?? ''),
});

const loading = computed(() => initialLoading.value);

watch([searchForApi, badgeFilter], () => {
  void loadInitial(true);
});

const allCards = computed(() => {
  const today = startOfToday();
  const weekEnd = new Date(today);
  weekEnd.setDate(weekEnd.getDate() + 7);
  const monthEnd = new Date(today);
  monthEnd.setMonth(monthEnd.getMonth() + 1);

  return posts.value
    .map(toCardModel)
    .filter((e) => {
      if (datePeriod.value === '_all') return true;
      const d = parseEventDate(e.rawDate);
      if (!d) return false;
      if (datePeriod.value === 'week') return d >= today && d <= weekEnd;
      return d >= today && d <= monthEnd;
    })
    .sort((a, b) => String(a.rawDate ?? '').localeCompare(String(b.rawDate ?? '')));
});

const upcomingCards = computed(() => allCards.value.filter((e) => !e.isPast));
const pastCards = computed(() =>
  [...allCards.value]
    .filter((e) => e.isPast)
    .sort((a, b) => String(b.rawDate ?? '').localeCompare(String(a.rawDate ?? ''))),
);
const myCards = computed(() => allCards.value.filter((e) => e.isJoined));

const tabItems = computed<TabsItem[]>(() => [
  {
    label: 'Все мероприятия',
    value: 'all',
    badge:
      upcomingCards.value.length || pastCards.value.length
        ? String(upcomingCards.value.length + pastCards.value.length)
        : undefined,
  },
  {
    label: 'Мои мероприятия',
    value: 'my',
    badge: myCards.value.length ? String(myCards.value.length) : undefined,
  },
]);

/** Все запланированные — в блоке «Ближайшие» на вкладке «Все». */
const plannedEvents = computed(() =>
  eventsTab.value === 'all' ? upcomingCards.value : [],
);

/** Прошедшие — под запланированными на вкладке «Все». */
const pastSectionEvents = computed(() =>
  eventsTab.value === 'all' ? pastCards.value : [],
);

/** Сетка для вкладки «Мои». */
const myGridEvents = computed(() =>
  eventsTab.value === 'my' ? myCards.value : [],
);

const showEmptyAll = computed(
  () =>
    eventsTab.value === 'all' &&
    !plannedEvents.value.length &&
    !pastSectionEvents.value.length,
);

const showEmptyMy = computed(
  () => eventsTab.value === 'my' && !myGridEvents.value.length,
);

const listForSentinel = computed(() => {
  if (eventsTab.value === 'my') return myCards.value;
  return [...upcomingCards.value, ...pastCards.value];
});
const headerLinks = computed(() => {
  if (!canEditSection('events') || isKiosk.value) return [];
  return [
    {
      label: 'Добавить мероприятие',
      color: 'neutral',
      variant: 'outline',
      size: 'md',
      onClick: openCreate,
    },
  ];
});

const eventPlaceholder = (() => {
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="450" viewBox="0 0 800 450">` +
    `<rect width="800" height="450" fill="#1e293b"/>` +
    `<text x="50%" y="50%" dominant-baseline="middle" text-anchor="middle" ` +
    `fill="#94a3b8" font-family="sans-serif" font-size="64" font-weight="700" letter-spacing="6">СОБЫТИЕ</text>` +
    `</svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
})();

function eventCoverSrc(event: EventCardModel): string {
  const src = event.image && typeof event.image === 'object' ? event.image.src : '';
  return src || eventPlaceholder;
}

// ─── Create form ──────────────────────────────────────────────────────────────
const createOpen = ref(false);

type CreateFormState = {
  title: string;
  description: string;
  badge: string | null;
  date: string;
  albumId: string;
  timeStart: string;
  timeEnd: string;
  location: string;
  maxParticipants: number | null;
};

const createState = reactive<CreateFormState>({
  title: '',
  description: '',
  badge: null,
  date: '',
  albumId: 'none',
  timeStart: '',
  timeEnd: '',
  location: '',
  maxParticipants: null,
});

const createImageFile = ref<File | null>(null);
const createImagePreview = ref('');

function onCreateImageSelected(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0] ?? null;
  createImageFile.value = file;
  if (createImagePreview.value) URL.revokeObjectURL(createImagePreview.value);
  createImagePreview.value = file ? URL.createObjectURL(file) : '';
}

type SingleDateValue = { value?: any } | any | null;
const createDateValue = ref<SingleDateValue>(null);
const createSubmitting = ref(false);
const createError = ref<string | null>(null);

watch(createDateValue, (val) => {
  const d = val?.value ?? val;
  createState.date = d && typeof d.toString === 'function' ? d.toString() : '';
});

function resetCreateForm() {
  createState.title = '';
  createState.description = '';
  createState.badge = null;
  createState.date = '';
  createState.albumId = 'none';
  createState.timeStart = '';
  createState.timeEnd = '';
  createState.location = '';
  createState.maxParticipants = null;
  createImageFile.value = null;
  if (createImagePreview.value) URL.revokeObjectURL(createImagePreview.value);
  createImagePreview.value = '';
  createDateValue.value = null;
  createError.value = null;
}

function openCreate() {
  ensureAlbumsLoaded();
  resetCreateForm();
  createOpen.value = true;
}

function validateCreate(): boolean {
  if (!createState.title.trim()) {
    createError.value = 'Заполните название мероприятия.';
    return false;
  }
  if (!createState.date) {
    createError.value = 'Выберите дату проведения.';
    return false;
  }
  if (createState.timeStart && createState.timeEnd && createState.timeEnd < createState.timeStart) {
    createError.value = 'Время окончания раньше начала.';
    return false;
  }
  createError.value = null;
  return true;
}

async function handleCreateSubmit() {
  if (!validateCreate()) return;
  createSubmitting.value = true;
  createError.value = null;
  try {
    const defaultImg = '/favicon.svg';
    let imagePath = defaultImg;
    let imageFullPath = defaultImg;

    if (createImageFile.value) {
      const formData = new FormData();
      formData.append('image', createImageFile.value);
      const uploadRes = await fetch('/api/Upload/upload.php', { method: 'POST', body: formData });
      const uploadJson = await uploadRes.json();
      if (!uploadJson.success) throw new Error(uploadJson.message || 'Ошибка загрузки изображения');
      imagePath = uploadJson.data.image;
      imageFullPath = uploadJson.data.image_full;
    }

    const json = await apiSessionFetch('/api/events.php', {
      method: 'POST',
      json: {
        title: createState.title.trim(),
        description: eventDescriptionBody(createState.description),
        badge: createState.badge || null,
        date: createState.date,
        image: imagePath,
        image_full: imageFullPath,
        album_id: createState.albumId && createState.albumId !== 'none' ? Number(createState.albumId) : null,
        ...eventScheduleBody(createState),
      },
    });
    if (!json.success) throw new Error(json.message || 'Ошибка создания');

    createOpen.value = false;
    resetCreateForm();
    await refresh();
  } catch (e: any) {
    createError.value = e.message ?? 'Ошибка при создании мероприятия';
  } finally {
    createSubmitting.value = false;
  }
}

const mainScrollEl = ref<HTMLElement | null>(null);
const eventsSentinelEl = ref<HTMLElement | null>(null);

useFeedSentinel({
  root: mainScrollEl,
  sentinel: eventsSentinelEl,
  enabled: sentinelEnabled,
  onIntersect: () => {
    void loadMore();
  },
  rootMargin: '600px 0px',
});

onMounted(() => {
  void loadInitial();
  void ensureRsvpLoaded();
});

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer);
});
</script>

<template>
  <UMain class="relative w-full h-full min-h-0">
    <div
      ref="mainScrollEl"
      class="flex flex-col w-full h-full min-h-0 gap-6 overflow-y-auto scrollbar-hide p-px"
    >
      <div class="flex flex-col gap-6 w-full max-w-[1600px] mx-auto pb-8">
        <UPageHeader
          headline="Корпоративная жизнь"
          title="Мероприятия"
          description="Рабочие, образовательные и корпоративные события"
          :links="headerLinks"
        />

        <div
          v-if="!isKiosk"
          class="flex flex-col sm:flex-row gap-3 items-stretch sm:items-center"
        >
          <UInput
            v-model="searchQuery"
            icon="i-lucide-search"
            size="md"
            color="neutral"
            variant="outline"
            placeholder="Найти мероприятие..."
            class="w-full min-w-0 flex-1"
          />
          <USelect
            v-model="datePeriod"
            :items="datePeriodOptions"
            value-key="value"
            size="md"
            color="neutral"
            class="w-full sm:w-52 shrink-0"
          />
          <USelect
            v-model="badgeFilter"
            :items="typeFilterOptions"
            value-key="value"
            size="md"
            color="neutral"
            class="w-full sm:w-40 shrink-0"
          />
        </div>

        <UTabs
          v-if="!isKiosk"
          v-model="eventsTab"
          :items="tabItems"
          variant="link"
          color="primary"
          size="md"
          :content="false"
          class="w-full border-b border-default"
          :ui="{
            list: 'w-full gap-1',
            trigger: 'grow-0',
          }"
        />

        <UAlert
          v-if="fetchError"
          color="error"
          variant="subtle"
          icon="i-lucide-alert-circle"
          :description="fetchError"
        >
          <template #footer>
            <UButton size="sm" color="error" variant="ghost" @click="refresh">
              Повторить
            </UButton>
          </template>
        </UAlert>

        <div v-else-if="loading" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          <USkeleton v-for="i in 6" :key="i" class="h-64 rounded-panel" />
        </div>

        <template v-else>
          <UEmpty
            v-if="showEmptyAll || showEmptyMy"
            variant="naked"
            icon="i-lucide-calendar-x"
            title="Мероприятия не найдены"
            description="Попробуйте изменить поиск или фильтры."
            class="w-full py-10"
          />

          <!-- Все запланированные -->
          <section
            v-if="plannedEvents.length"
            class="flex flex-col gap-4"
            aria-label="Ближайшие мероприятия"
          >
            <h2 class="text-lg font-semibold text-highlighted">Ближайшие</h2>
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">
              <article
                v-for="event in plannedEvents"
                :key="`planned-${event.id ?? event.title}`"
                class="group flex flex-col sm:flex-row gap-3 rounded-panel p-px transition ring-1 ring-default hover:ring-primary"
              >
                <RouterLink
                  :to="event.to || `/events/${event.id}`"
                  class="relative aspect-[16/10] sm:aspect-auto sm:w-44 sm:min-h-[140px] shrink-0 overflow-hidden rounded-panel bg-elevated"
                >
                  <img
                    :src="eventCoverSrc(event)"
                    :alt="event.title"
                    loading="lazy"
                    decoding="async"
                    class="size-full object-cover transition duration-300 group-hover:scale-[1.02]"
                  />
                  <span
                    class="absolute bottom-2 left-2 inline-flex flex-col items-center rounded-md bg-black/65 px-2 py-1 text-white backdrop-blur-sm"
                  >
                    <span class="text-lg font-semibold leading-none tabular-nums">{{ event.day }}</span>
                    <span class="text-[10px] font-medium uppercase tracking-wide">{{ event.monthShort }}</span>
                  </span>
                </RouterLink>

                <div class="flex min-w-0 flex-1 flex-col gap-3 px-1 py-1 sm:pr-2 sm:py-2">
                  <RouterLink :to="event.to || `/events/${event.id}`" class="flex min-w-0 flex-col gap-1.5">
                    <h3 class="text-base font-semibold text-highlighted line-clamp-2 text-pretty">
                      {{ event.title }}
                    </h3>
                    <p class="inline-flex items-center gap-1.5 text-sm text-muted min-w-0">
                      <UIcon name="i-lucide-clock" class="size-3.5 shrink-0" />
                      <span class="truncate">{{ event.timeLabel }}</span>
                    </p>
                    <p class="inline-flex items-center gap-1.5 text-sm text-muted min-w-0">
                      <UIcon name="i-lucide-map-pin" class="size-3.5 shrink-0" />
                      <span class="truncate">{{ event.placeLabel }}</span>
                    </p>
                    <p
                      class="inline-flex items-center gap-1.5 text-sm min-w-0"
                      :class="event.isFull ? 'text-warning' : 'text-muted'"
                    >
                      <UIcon name="i-lucide-users" class="size-3.5 shrink-0" aria-hidden="true" />
                      <span class="truncate">{{ event.seatsLabel }}</span>
                    </p>
                  </RouterLink>
                  <div v-if="!isKiosk" class="mt-auto flex flex-wrap gap-2">
                    <UButton
                      color="primary"
                      variant="outline"
                      size="md"
                      :icon="event.isJoined ? 'i-lucide-check' : event.isFull ? 'i-lucide-lock' : 'i-lucide-calendar-plus'"
                      :disabled="event.isArchived || (event.isFull && !event.isJoined)"
                      @click="toggleJoin(event, $event)"
                    >
                      {{ event.isJoined ? 'Вы записаны' : event.isFull ? 'Мест нет' : 'Записаться' }}
                    </UButton>
                    <UButton
                      color="neutral"
                      variant="outline"
                      size="md"
                      :to="event.to || `/events/${event.id}`"
                    >
                      Подробнее
                    </UButton>
                  </div>
                </div>
              </article>
            </div>
          </section>

          <!-- Прошедшие (на вкладке «Все») -->
          <section
            v-if="pastSectionEvents.length"
            class="flex flex-col gap-4"
            aria-label="Прошедшие мероприятия"
          >
            <h2 class="text-lg font-semibold text-highlighted">Прошедшие мероприятия</h2>
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              <RouterLink
                v-for="event in pastSectionEvents"
                :key="`past-${event.id ?? event.title}`"
                :to="event.to || `/events/${event.id}`"
                class="group flex flex-col gap-3 rounded-panel p-px transition ring-1 ring-transparent hover:ring-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              >
                <div class="relative aspect-[16/10] overflow-hidden rounded-panel bg-elevated">
                  <img
                    :src="eventCoverSrc(event)"
                    :alt="event.title"
                    loading="lazy"
                    decoding="async"
                    class="size-full object-cover opacity-80 transition duration-300 group-hover:scale-[1.02]"
                  />
                  <span
                    class="absolute bottom-2 left-2 inline-flex flex-col items-center rounded-md bg-black/65 px-2 py-1 text-white backdrop-blur-sm"
                  >
                    <span class="text-base font-semibold leading-none tabular-nums">{{ event.day }}</span>
                    <span class="text-[10px] font-medium uppercase tracking-wide">{{ event.monthShort }}</span>
                  </span>
                  <UBadge
                    v-if="event.badge"
                    class="absolute top-2 right-2"
                    color="neutral"
                    variant="solid"
                    size="sm"
                  >
                    {{ event.badge }}
                  </UBadge>
                </div>
                <div class="flex min-w-0 flex-col gap-1 px-0.5">
                  <p v-if="event.date" class="text-sm text-muted">{{ event.date }}</p>
                  <h3 class="text-base font-semibold text-highlighted line-clamp-2 text-pretty">
                    {{ event.title }}
                  </h3>
                  <p class="text-xs text-muted line-clamp-1">
                    {{ event.timeLabel }} · {{ event.placeLabel }}
                  </p>
                </div>
              </RouterLink>
            </div>
          </section>

          <!-- Мои мероприятия -->
          <section
            v-if="myGridEvents.length"
            class="flex flex-col gap-4"
            aria-label="Мои мероприятия"
          >
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              <div
                v-for="event in myGridEvents"
                :key="`my-${event.id ?? event.title}`"
                class="group flex flex-col gap-3 rounded-panel p-px transition ring-1 ring-transparent hover:ring-primary"
              >
                <RouterLink
                  :to="event.to || `/events/${event.id}`"
                  class="flex flex-col gap-3 focus-visible:outline-none"
                >
                  <div class="relative aspect-[16/10] overflow-hidden rounded-panel bg-elevated">
                    <img
                      :src="eventCoverSrc(event)"
                      :alt="event.title"
                      loading="lazy"
                      decoding="async"
                      class="size-full object-cover transition duration-300 group-hover:scale-[1.02]"
                      :class="event.isPast ? 'opacity-80' : ''"
                    />
                    <span
                      class="absolute bottom-2 left-2 inline-flex flex-col items-center rounded-md bg-black/65 px-2 py-1 text-white backdrop-blur-sm"
                    >
                      <span class="text-base font-semibold leading-none tabular-nums">{{ event.day }}</span>
                      <span class="text-[10px] font-medium uppercase tracking-wide">{{ event.monthShort }}</span>
                    </span>
                  </div>
                  <div class="flex min-w-0 flex-col gap-1 px-0.5">
                    <p v-if="event.date" class="text-sm text-muted">{{ event.date }}</p>
                    <h3 class="text-base font-semibold text-highlighted line-clamp-2 text-pretty">
                      {{ event.title }}
                    </h3>
                    <p class="text-xs text-muted line-clamp-1">
                      {{ event.timeLabel }} · {{ event.placeLabel }}
                    </p>
                  </div>
                </RouterLink>
                <div v-if="!isKiosk && !event.isPast" class="px-0.5 pb-0.5">
                  <UButton
                    color="neutral"
                    variant="soft"
                    size="md"
                    block
                    icon="i-lucide-check"
                    @click="toggleJoin(event, $event)"
                  >
                    Вы записаны
                  </UButton>
                </div>
              </div>
            </div>
          </section>

          <div
            v-if="feedLoading && !loading"
            class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3"
          >
            <USkeleton v-for="i in 3" :key="`more-${i}`" class="h-64 rounded-panel" />
          </div>

          <div
            v-if="listForSentinel.length"
            ref="eventsSentinelEl"
            class="h-1 w-full shrink-0"
            aria-hidden="true"
          />
        </template>
      </div>
    </div>

    <USlideover v-model:open="createOpen" side="right" title="Новое мероприятие" description="">
      <template #body>
        <UForm :state="createState" class="space-y-4" @submit.prevent="handleCreateSubmit">
          <UFormField label="Название мероприятия" name="title" required>
            <UInput
              v-model="createState.title"
              size="xl"
              class="w-full"
              placeholder="Введите название мероприятия"
            />
          </UFormField>

          <UFormField label="Категория" name="badge">
            <USelect
              v-model="createState.badge"
              :content="slideoverSelectContent"
              :items="createBadgeOptions"
              placeholder="Выберите: Новое или Архив"
              size="xl"
              class="w-full"
            />
          </UFormField>

          <UFormField label="Описание" name="description">
            <EventDescriptionEditor v-model="createState.description" />
          </UFormField>

          <UFormField label="Дата проведения" name="date" required>
            <UInputDate v-model="createDateValue" size="xl" class="w-full">
              <template #trailing>
                <UTooltip text="Выбрать дату">
                  <span class="inline-flex">
                    <UPopover :content="slideoverPopoverContent">
                      <UButton
                        color="neutral"
                        variant="link"
                        size="md"
                        icon="i-lucide-calendar"
                        aria-label="Выбрать дату"
                        class="px-0"
                      />
                      <template #content>
                        <UCalendar v-model="createDateValue" class="p-2" />
                      </template>
                    </UPopover>
                  </span>
                </UTooltip>
              </template>
            </UInputDate>
          </UFormField>

          <EventScheduleFields
            v-model:time-start="createState.timeStart"
            v-model:time-end="createState.timeEnd"
            v-model:location="createState.location"
            v-model:max-participants="createState.maxParticipants"
            size="xl"
          />

          <UFormField label="Альбом фотогалереи" name="albumId">
            <USelect
              v-model="createState.albumId"
              :content="slideoverSelectContent"
              :items="albumSelectItems"
              placeholder="Выберите альбом"
              size="xl"
              class="w-full"
            />
          </UFormField>

          <UFormField label="Изображение" name="image">
            <div class="flex flex-col gap-2 w-full">
              <input
                id="createFileInput"
                type="file"
                accept="image/jpeg,image/png,image/webp"
                class="hidden"
                @change="onCreateImageSelected"
              />
              <img
                v-if="createImagePreview"
                :src="createImagePreview"
                alt="Предпросмотр"
                class="w-full h-40 object-cover rounded-lg"
              />
              <label
                for="createFileInput"
                class="flex items-center justify-center gap-2 cursor-pointer rounded-lg border border-default px-4 py-2.5 text-sm font-medium text-default hover:bg-elevated/50 transition-colors"
              >
                <UIcon name="i-lucide-upload" class="text-base shrink-0" />
                {{ createImageFile ? 'Изменить фото' : 'Загрузить фото' }}
              </label>
              <p v-if="createImageFile" class="text-xs text-muted truncate px-1">
                {{ createImageFile.name }}
              </p>
            </div>
          </UFormField>

          <UAlert
            v-if="createError"
            color="error"
            variant="subtle"
            icon="i-lucide-alert-circle"
            :description="createError"
          />
        </UForm>
      </template>
      <template #footer>
        <div class="flex justify-between gap-3 items-center w-full">
          <UButton
            color="neutral"
            variant="outline"
            size="md"
            class="w-full justify-center"
            @click="createOpen = false"
          >
            Отмена
          </UButton>
          <UButton
            size="md"
            class="w-full justify-center"
            :loading="createSubmitting"
            @click="handleCreateSubmit"
          >
            Создать
          </UButton>
        </div>
      </template>
    </USlideover>

    <ScrollToTopButton :target="mainScrollEl" />
  </UMain>
</template>

<style>
:global([data-reka-select-content]),
:global([data-reka-popover-content]),
:global([data-reka-combobox-content]) {
  z-index: 100 !important;
}
</style>
