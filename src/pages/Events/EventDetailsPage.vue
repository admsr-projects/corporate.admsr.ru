<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted, onUnmounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import type { BlogPostProps } from '@nuxt/ui';
import { useSectionAccess } from '../../composables/useSectionAccess';
import { apiSessionFetch } from '../../composables/useAuthSession';
import { useAppToast } from '../../composables/useAppToast';
import { useEventRsvp } from '../../composables/useEventRsvp';
import { useGalleryData } from '../../composables/useGalleryData';
import { slideoverPopoverContent, slideoverSelectContent } from '../../composables/slideoverFieldUi';
import { toCalendarDate } from '../../utils/date';
import { userAvatarSrc } from '../../utils/userName';
import {
  eventScheduleBody,
  eventSeatsLabel,
  eventTimeLabel,
  isEventFull,
  readEventSchedule,
  type EventSchedule,
} from '../../composables/eventSchedule';
import EventScheduleFields from './components/EventScheduleFields.vue';

type EventPost = BlogPostProps & {
  id?: number;
  badge?: string;
  image_full?: string;
  link?: string;
  album_id?: number | null;
  album_name?: string | null;
  schedule: EventSchedule;
};

type EventParticipant = { id: number; name: string; avatar_url: string; joinedAt: string };

type AlbumPhoto = {
  id: string;
  thumbSrc: string;
  fullSrc: string;
};

type AlbumSlide =
  | (AlbumPhoto & { type: 'photo' })
  | { type: 'cta'; id: 'album-cta' };

const route = useRoute();
const router = useRouter();

// ─── State ───────────────────────────────────────────────────────────────────
const event = ref<EventPost | null>(null);
const loading = ref(false);
const fetchError = ref<string | null>(null);

const { canEditSection, ensureLoaded: ensureSectionAccess } = useSectionAccess();
ensureSectionAccess();
const { albums, ensureLoaded: ensureAlbumsLoaded } = useGalleryData();
ensureAlbumsLoaded();

const albumSelectItems = computed(() => [
  { label: 'Без альбома', value: 'none' },
  ...albums.value.map((a) => ({ label: a.title, value: String(a.id) })),
]);
const isAdmin = computed(() => canEditSection('events'));
const isKiosk = computed(() => route.matched?.some((r) => r.meta?.kiosk));
const isArchivedEvent = computed(() => {
  const badge = String(event.value?.badge ?? '').trim().toLowerCase();
  return badge.includes('архив');
});
const isNewEvent = computed(() => {
  const badge = String(event.value?.badge ?? '').trim().toLowerCase();
  return badge.includes('нов');
});

const { toast } = useAppToast();

const editBadgeOptions = [
  { value: 'Новое', label: 'Новое' },
  { value: 'Архив', label: 'Архив' },
];

// ─── API ─────────────────────────────────────────────────────────────────────
function firstSentence(text: string): string {
  if (!text) return text;
  return text.match(/^[^.!?]*[.!?]/)?.[0].trim() ?? text;
}

/** Дата для отображения: «1 апреля 2025 г.» (ru-RU) */
function formatDateRu(raw: string | Date | undefined): string {
  if (raw == null) return '';
  if (raw instanceof Date) {
    if (Number.isNaN(raw.getTime())) return '';
    return new Intl.DateTimeFormat('ru-RU', {
      day: 'numeric',
      month: 'long',
      year: 'numeric',
    }).format(raw);
  }
  const s = raw.trim();
  if (!s) return '';
  const iso = /^(\d{4})-(\d{2})-(\d{2})/.exec(s);
  const d = iso
    ? new Date(Number(iso[1]), Number(iso[2]) - 1, Number(iso[3]))
    : new Date(s);
  if (Number.isNaN(d.getTime())) return s;
  return new Intl.DateTimeFormat('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  }).format(d);
}

function mapEvent(raw: any): EventPost {
  return {
    id:          raw.id,
    title:       raw.title,
    description: raw.description ?? '',
    badge:       raw.badge ?? undefined,
    date:        raw.date,
    to:          `/events/${raw.id}`,
    image:       raw.image      ? { src: raw.image,      alt: raw.title } : undefined,
    image_full:  raw.image_full || raw.image || '',
    link:        raw.link ?? undefined,
    album_id:    raw.album_id != null ? Number(raw.album_id) : null,
    album_name:  raw.album_name ?? null,
    schedule:    readEventSchedule(raw),
  };
}

function isVideoUrl(url: string): boolean {
  return /\.mp4(\?|$)/i.test(url ?? '');
}

const albumPhotos = ref<AlbumPhoto[]>([]);
const albumPhotosLoading = ref(false);

const galleryAlbumTo = computed(() => {
  const albumId = event.value?.album_id;
  if (!albumId) return '';
  return isKiosk.value ? `/kiosk/gallery/${albumId}` : `/gallery/${albumId}`;
});

/** До 10 фото + финальная карточка «Смотреть в альбоме» */
const albumSlides = computed<AlbumSlide[]>(() => {
  const photos = albumPhotos.value.slice(0, 10).map((p) => ({ ...p, type: 'photo' as const }));
  if (!photos.length) return [];
  return [...photos, { type: 'cta' as const, id: 'album-cta' }];
});

async function loadAlbumPhotos(albumId?: number | null) {
  if (!albumId) {
    albumPhotos.value = [];
    return;
  }
  albumPhotosLoading.value = true;
  try {
    const res = await fetch(`/api/gallery_base.php?album_id=${albumId}`);
    const json = await res.json();
    albumPhotos.value = json.success
      ? (json.data ?? []).map((p: any) => ({
          id: String(p.id),
          thumbSrc: p.image_small_url,
          fullSrc: p.image_full_url,
        }))
      : [];
  } catch {
    albumPhotos.value = [];
  } finally {
    albumPhotosLoading.value = false;
  }
}

async function fetchEvent(id: string | string[]) {
  loading.value = true;
  fetchError.value = null;
  event.value = null;
  try {
    const res = await fetch(`/api/events.php?id=${id}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);

    const json = await res.json();
    if (!json.success) throw new Error(json.message || 'Ошибка загрузки');

    event.value = mapEvent(json.data);
    void loadParticipants();
    await loadAlbumPhotos(event.value.album_id);
  } catch (e: any) {
    fetchError.value = e.message ?? 'Не удалось загрузить мероприятие';
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void fetchEvent(route.params.id);
  void ensureRsvpLoaded();
});
watch(() => route.params.id, fetchEvent);

watch(
  fetchError,
  (val) => {
    if (!val) return;
    toast.add({
      title: 'Не удалось загрузить мероприятие',
      description: String(val),
      color: 'error',
      icon: 'i-lucide-alert-circle',
    });
  },
  { immediate: true },
);

// ─── Запись на мероприятие (сервер, V16) ──────────────────────────────────────
const { ensureLoaded: ensureRsvpLoaded, isJoined: isJoinedEvent, setJoined } = useEventRsvp();
const isJoined = computed(() => isJoinedEvent(event.value?.id));
/** Лимит мест заполнен (V22) — записаться нельзя, отменить свою запись можно. */
const isFull = computed(() => !!event.value && isEventFull(event.value.schedule));
const timeLabel = computed(() => (event.value ? eventTimeLabel(event.value.schedule) : ''));
const seatsLabel = computed(() => (event.value ? eventSeatsLabel(event.value.schedule) : ''));

// ─── Записавшиеся — только организаторам (правка раздела «Мероприятия», ADR-054) ───
const participants = ref<EventParticipant[]>([]);
const participantsLoading = ref(false);
const participantsError = ref<string | null>(null);
const showParticipants = computed(() => isAdmin.value && !isKiosk.value);

async function loadParticipants() {
  const id = event.value?.id;
  if (!id || !showParticipants.value) return;
  participantsLoading.value = true;
  participantsError.value = null;
  try {
    const json = await apiSessionFetch<EventParticipant[]>(`/api/event_rsvp.php?eventId=${id}`);
    if (!json?.success) throw new Error(json?.message || 'Не удалось загрузить список');
    if (event.value?.id !== id) return;
    participants.value = Array.isArray(json.data) ? json.data : [];
  } catch (e) {
    participantsError.value = e instanceof Error ? e.message : 'Не удалось загрузить список';
  } finally {
    participantsLoading.value = false;
  }
}

// Права раздела приходят после страницы — тогда и грузим список.
watch(showParticipants, (v) => {
  if (v) void loadParticipants();
});

function formatJoinedAt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' }).format(d);
}

async function toggleJoin() {
  const id = event.value?.id;
  if (!id) return;
  if (isArchivedEvent.value) return;
  const wasJoined = isJoined.value;
  if (!wasJoined && isFull.value) return;
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
  if (event.value?.id === id && res.participantsCount != null) {
    event.value.schedule = { ...event.value.schedule, participantsCount: res.participantsCount };
  }
  void loadParticipants();
  toast.add({
    title: wasJoined ? 'Запись отменена' : 'Вы записались на мероприятие',
    color: 'success',
    icon: 'i-lucide-circle-check',
  });
}

// ─── Share / Calendar helpers ────────────────────────────────────────────────
async function copyEventLink() {
  if (typeof window === 'undefined') return;
  const url = window.location.href;
  try {
    await navigator.clipboard.writeText(url);
    toast.add({
      title: 'Ссылка скопирована',
      description: 'Можно отправлять или открывать на другом устройстве.',
      color: 'success',
      icon: 'i-lucide-circle-check',
    });
  } catch {
    toast.add({
      title: 'Не удалось скопировать ссылку',
      description: 'Браузер мог запретить доступ к буферу обмена.',
      color: 'error',
      icon: 'i-lucide-alert-circle',
    });
  }
}

// ─── Форма редактирования ─────────────────────────────────────────────────────
type EditFormState = {
  title: string;
  description: string;
  badge: string;
  date: string;
  albumId: string;
  timeStart: string;
  timeEnd: string;
  location: string;
  maxParticipants: number | null;
};

const editOpen = ref(false);
const editState = reactive<EditFormState>({
  title: '',
  description: '',
  badge: '',
  date: '',
  albumId: 'none',
  timeStart: '',
  timeEnd: '',
  location: '',
  maxParticipants: null,
});

const editImageFile = ref<File | null>(null);
const editImagePreview = ref('');

function onEditImageSelected(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0] ?? null;
  editImageFile.value = file;
  if (editImagePreview.value) URL.revokeObjectURL(editImagePreview.value);
  editImagePreview.value = file ? URL.createObjectURL(file) : '';
}

type SingleDateValue = { value?: any } | any | null;
const editDateValue = ref<SingleDateValue>(null);
const editSubmitting = ref(false);
const editError = ref<string | null>(null);

function fillEditState() {
  if (!event.value) return;
  editState.title       = event.value.title ?? '';
  editState.description = event.value.description ?? '';
  editState.badge       = String((event.value as any).badge ?? '');
  editState.date        = (event.value as any).date ?? '';
  editState.albumId     = event.value.album_id ? String(event.value.album_id) : 'none';
  editState.timeStart   = event.value.schedule.timeStart;
  editState.timeEnd     = event.value.schedule.timeEnd;
  editState.location    = event.value.schedule.location;
  editState.maxParticipants = event.value.schedule.maxParticipants;
  editDateValue.value   = toCalendarDate(editState.date);
  editImageFile.value = null;
}

function openEdit() {
  ensureAlbumsLoaded();
  fillEditState();
  editOpen.value = true;
}

watch(editDateValue, (val) => {
  const d = val?.value ?? val;
  editState.date = d && typeof d.toString === 'function' ? d.toString() : '';
});

function validateEdit(): boolean {
  if (!editState.title.trim()) {
    editError.value = 'Заполните название мероприятия.';
    return false;
  }
  if (!editState.date) {
    editError.value = 'Выберите дату проведения.';
    return false;
  }
  if (editState.timeStart && editState.timeEnd && editState.timeEnd < editState.timeStart) {
    editError.value = 'Время окончания раньше начала.';
    return false;
  }
  editError.value = null;
  return true;
}

async function handleEditSubmit() {
  if (!validateEdit() || !event.value?.id) return;
  editSubmitting.value = true;
  editError.value = null;
  try {
    let imagePatch: { image?: string; image_full?: string } = {};
    if (editImageFile.value) {
      const formData = new FormData();
      formData.append('image', editImageFile.value);
      const uploadRes = await fetch('/api/Upload/upload.php', { method: 'POST', body: formData });
      const uploadJson = await uploadRes.json();
      if (!uploadJson.success) throw new Error(uploadJson.message || 'Ошибка загрузки изображения');
      imagePatch = { image: uploadJson.data.image, image_full: uploadJson.data.image_full };
    }

    const json = await apiSessionFetch(`/api/events.php?id=${event.value.id}`, {
      method: 'PUT',
      json: {
        title:       editState.title.trim(),
        description: editState.description.trim() || null,
        badge:       editState.badge || null,
        date:        editState.date,
        album_id:    editState.albumId && editState.albumId !== 'none' ? Number(editState.albumId) : null,
        ...eventScheduleBody(editState),
        ...imagePatch,
      },
    });
    if (!json.success) throw new Error(json.message || 'Ошибка сохранения');

    event.value = mapEvent(json.data);
    await loadAlbumPhotos(event.value.album_id);
    editOpen.value = false;
  } catch (e: any) {
    editError.value = e.message ?? 'Ошибка при сохранении';
  } finally {
    editSubmitting.value = false;
  }
}

// ─── Удаление ─────────────────────────────────────────────────────────────────
const deleteConfirmOpen = ref(false);
const deleteSubmitting = ref(false);
const deleteError = ref<string | null>(null);

watch(editError, (val) => {
  if (!val) return;
  toast.add({
    title: 'Не удалось сохранить',
    description: String(val),
    color: 'error',
    icon: 'i-lucide-alert-circle',
  });
});

watch(deleteError, (val) => {
  if (!val) return;
  toast.add({
    title: 'Не удалось удалить',
    description: String(val),
    color: 'error',
    icon: 'i-lucide-alert-circle',
  });
});

async function handleDelete() {
  if (!event.value?.id) return;
  deleteSubmitting.value = true;
  deleteError.value = null;
  try {
    const json = await apiSessionFetch(`/api/events.php?id=${event.value.id}`, {
      method: 'DELETE',
    });
    if (!json.success) throw new Error(json.message || 'Ошибка удаления');

    deleteConfirmOpen.value = false;
    router.push({ name: 'events' });
  } catch (e: any) {
    deleteError.value = e.message ?? 'Ошибка при удалении';
  } finally {
    deleteSubmitting.value = false;
  }
}

const mainScrollEl = ref<HTMLElement | null>(null);
const showFloatingHeader = ref(false);
let lastScrollTop = 0;
let rafPending = false;
const FLOATING_SHOW_AT = 220;
const FLOATING_HIDE_AT = 140;

function onMainScroll() {
  const el = mainScrollEl.value;
  if (!el) return;
  if (rafPending) return;
  rafPending = true;
  requestAnimationFrame(() => {
    rafPending = false;
    const top = el.scrollTop;
    const goingUp = top < lastScrollTop;
    const goingDown = top > lastScrollTop;

    if (top < FLOATING_HIDE_AT) {
      showFloatingHeader.value = false;
    } else if (goingUp && top > FLOATING_SHOW_AT) {
      showFloatingHeader.value = true;
    } else if (goingDown) {
      showFloatingHeader.value = false;
    }

    lastScrollTop = top;
  });
}

onMounted(() => {
  const el = mainScrollEl.value;
  if (!el) return;
  lastScrollTop = el.scrollTop;
  el.addEventListener('scroll', onMainScroll, { passive: true });
});

onUnmounted(() => {
  const el = mainScrollEl.value;
  if (el) el.removeEventListener('scroll', onMainScroll);
});
</script>

<template>
  <UMain class="relative flex flex-col w-full h-full min-h-0 gap-0">
    <div ref="mainScrollEl" class="flex-1 min-h-0 overflow-y-auto max-w-full w-full sm:p-0 md:p-0 lg:p-0 xl:p-0 scrollbar-hide mx-0 pb-8">
      <!-- Floating header overlay -->
      <div v-if="!isKiosk" class="sticky top-0 z-30 h-0">
        <div
          :class="[
            'absolute left-0 right-0',
            'transition duration-200 ease-out will-change-transform',
            showFloatingHeader ? 'opacity-100 translate-y-0 pointer-events-auto' : 'opacity-0 -translate-y-2 pointer-events-none',
          ]"
        >
          <UContainer class="max-w-full w-full sm:p-0 md:p-0 lg:p-0 xl:p-0 mx-0">
            <UContainer class="bg-default/85 backdrop-blur rounded-xl ring ring-default p-3">
              <UPageHeader title="" class="border-none p-0 w-full">
                <template #title>
                  <h1 class="text-xl sm:text-2xl font-semibold text-highlighted truncate">
                    {{ event?.title ?? 'Мероприятие' }}
                  </h1>
                </template>
                <template #description>
                  <p v-if="event?.description" class="text-sm text-muted line-clamp-2">
                    {{ firstSentence(event.description ?? '') }}
                  </p>
                </template>
              </UPageHeader>
            </UContainer>
          </UContainer>
        </div>
      </div>

      <UAlert v-if="fetchError" color="error" variant="subtle" icon="i-lucide-alert-circle" :title="fetchError" class="mb-6">
        <template #footer>
          <UButton size="sm" color="error" variant="ghost" @click="fetchEvent(route.params.id)">Повторить</UButton>
        </template>
      </UAlert>

      <!-- Скелетон -->
      <div v-if="loading" class="space-y-6">
        <USkeleton class="w-full rounded-lg" :class="isKiosk ? 'aspect-video' : 'h-56 sm:h-72'" />
        <div class="grid grid-cols-1 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)] gap-6 items-start">
          <USkeleton class="h-64 rounded-lg" />
          <USkeleton class="h-64 rounded-lg" />
        </div>
      </div>

      <!-- Контент -->
      <div v-else-if="event" class="flex flex-col gap-6 p-px">
        <!-- Hero -->
        <div class="relative overflow-hidden rounded-lg ring ring-default bg-default">
          <div class="relative w-full" :class="isKiosk ? 'aspect-video' : 'h-96'">
            <img
              v-if="event.image_full || (event.image as any)?.src"
              :src="event.image_full || (event.image as any)?.src"
              :alt="(event.image as any)?.alt ?? event.title"
              class="absolute inset-0 h-full w-full object-cover"
            />
            <div class="absolute inset-0 bg-linear-to-t from-black/65 via-black/25 to-transparent" />

            <div class="absolute inset-0 flex flex-col justify-end" :class="isKiosk ? 'p-8' : 'p-6'">
              <div class="flex flex-wrap items-center gap-2 mb-3">
                <UBadge v-if="event.badge" :color="isNewEvent ? 'primary' : 'neutral'" :variant="isNewEvent ? 'solid' : 'soft'" :size="isKiosk ? 'xl' : 'lg'">
                  {{ event.badge }}
                </UBadge>
                <UBadge color="neutral" variant="soft" :size="isKiosk ? 'xl' : 'lg'" class="backdrop-blur">
                  {{ formatDateRu(event.date) }}
                </UBadge>
              </div>

              <div class="flex items-end justify-between gap-4 flex-wrap">
                <div class="min-w-0">
                  <h1
                    class="font-semibold tracking-tight text-white"
                    :class="isKiosk ? 'text-5xl leading-tight' : 'text-2xl sm:text-4xl'"
                  >
                    {{ event.title }}
                  </h1>
                  <p
                    v-if="event.description"
                    class="mt-2 text-white/85 max-w-3xl"
                    :class="isKiosk ? 'text-2xl leading-relaxed' : 'text-sm sm:text-base'"
                  >
                    {{ firstSentence(event.description ?? '') }}
                  </p>
                </div>

                <div v-if="isAdmin && !isKiosk" class="flex gap-2">
                  <UButton color="neutral" variant="outline" size="lg" icon="i-lucide-pencil" @click="openEdit">
                    Изменить
                  </UButton>
                  <UButton color="error" variant="subtle" size="lg" icon="i-lucide-trash-2" @click="deleteConfirmOpen = true">
                    Удалить
                  </UButton>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Фотоальбом -->
        <div v-if="event.album_id && (albumPhotosLoading || albumPhotos.length)" class="flex flex-col gap-4">
          <div class="flex items-center justify-between gap-4">
            <h2
              class="font-semibold text-highlighted min-w-0 truncate"
              :class="isKiosk ? 'text-3xl font-unbounded' : 'text-xl'"
            >
              {{ event.album_name || 'Фотоальбом' }}
            </h2>
            <UButton
              :to="galleryAlbumTo"
              color="neutral"
              variant="outline"
              :size="isKiosk ? 'xl' : 'lg'"
              trailing-icon="i-lucide-arrow-right"
              class="shrink-0"
            >
              Смотреть все
            </UButton>
          </div>

          <USkeleton v-if="albumPhotosLoading" class="w-full rounded-2xl" :class="isKiosk ? 'h-80' : 'h-56'" />

          <UCarousel
            v-else
            v-slot="{ item }"
            :items="albumSlides"
            arrows
            dots
            wheel-gestures
            :autoplay="albumSlides.length > 1 ? { delay: 6000, stopOnInteraction: true } : false"
            :prev="{ color: 'neutral', variant: 'solid', size: isKiosk ? 'xl' : 'lg' }"
            :next="{ color: 'neutral', variant: 'solid', size: isKiosk ? 'xl' : 'lg' }"
            :ui="{
              root: 'relative w-full',
              item: 'basis-full sm:basis-4/5 ps-0',
              container: 'ms-0 gap-4',
              viewport: 'rounded-2xl',
              dots: 'relative mt-4 inset-x-auto bottom-auto flex flex-nowrap items-center justify-center gap-1.5',
              dot: 'size-2 shrink-0',
            }"
            class="w-full"
          >
            <RouterLink
              v-if="item.type === 'cta'"
              :to="galleryAlbumTo"
              class="flex h-full w-full aspect-video flex-col items-center justify-center gap-3 rounded-2xl ring ring-default bg-elevated/80 px-6 text-center transition-colors hover:bg-elevated"
            >
              <UIcon name="i-lucide-images" :class="isKiosk ? 'size-12 text-primary' : 'size-10 text-primary'" />
              <span
                class="font-semibold text-highlighted"
                :class="isKiosk ? 'text-2xl font-unbounded' : 'text-lg'"
              >
                Смотреть в альбоме
              </span>
              <span class="text-muted" :class="isKiosk ? 'text-base' : 'text-sm'">
                Все фотографии мероприятия
              </span>
            </RouterLink>

            <div
              v-else
              class="overflow-hidden rounded-2xl ring ring-default bg-elevated aspect-video w-full"
            >
              <video
                v-if="isVideoUrl(item.fullSrc)"
                :src="item.fullSrc"
                class="h-full w-full object-cover"
                muted
                loop
                playsinline
                preload="metadata"
              />
              <img
                v-else
                :src="item.fullSrc || item.thumbSrc"
                alt=""
                class="h-full w-full object-cover"
                loading="lazy"
              />
            </div>
          </UCarousel>
        </div>

        <!-- Body -->
        <div
          class="grid gap-6 items-start"
          :class="isKiosk ? 'grid-cols-1' : 'grid-cols-1 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]'"
        >
          <UCard class="rounded-lg" :class="isKiosk ? 'order-2' : ''">
            <template #header>
              <div class="flex items-center justify-between gap-3">
                <div class="flex items-center gap-2">
                  <UIcon name="i-lucide-info" :class="isKiosk ? 'size-7 text-primary' : 'size-5 text-primary'" />
                  <span :class="isKiosk ? 'text-xl font-semibold text-highlighted' : 'text-sm font-semibold text-highlighted'">О мероприятии</span>
                </div>
              </div>
            </template>

            <div class="space-y-4">
              <p
                v-if="event.description"
                class="leading-relaxed text-muted"
                :class="isKiosk ? 'text-3xl' : 'text-base leading-6'"
              >
                {{ event.description }}
              </p>
              <p v-else :class="isKiosk ? 'text-2xl text-muted' : 'text-sm text-muted'">
                Описание мероприятия пока не добавлено.
              </p>

              <div v-if="!isKiosk" class="flex flex-wrap gap-2 pt-1">
                <UButton
                  :color="isArchivedEvent || (isFull && !isJoined) ? 'neutral' : (isJoined ? 'neutral' : 'primary')"
                  :variant="isArchivedEvent || (isFull && !isJoined) ? 'outline' : (isJoined ? 'soft' : 'solid')"
                  size="lg"
                  :icon="isArchivedEvent || (isFull && !isJoined) ? 'i-lucide-lock' : (isJoined ? 'i-lucide-check' : 'i-lucide-ticket')"
                  :disabled="isArchivedEvent || (isFull && !isJoined)"
                  @click="toggleJoin"
                >
                  {{ isArchivedEvent ? 'Запись недоступна' : isJoined ? 'Вы записаны' : isFull ? 'Мест нет' : 'Записаться' }}
                </UButton>

                <UButton color="neutral" variant="outline" size="lg" icon="i-lucide-link" @click="copyEventLink">
                  Скопировать ссылку
                </UButton>
              </div>
            </div>
          </UCard>

          <div class="space-y-4" :class="isKiosk ? 'order-1' : ''">
            <UCard class="rounded-lg">
              <template #header>
                <div class="flex items-center gap-2">
                  <UIcon name="i-lucide-calendar-days" :class="isKiosk ? 'size-7 text-primary' : 'size-5 text-primary'" />
                  <span :class="isKiosk ? 'text-xl font-semibold text-highlighted' : 'text-sm font-semibold text-highlighted'">Детали</span>
                </div>
              </template>

              <dl class="grid grid-cols-1 gap-3">
                <div class="flex items-start justify-between gap-3">
                  <dt :class="isKiosk ? 'text-xl text-muted' : 'text-sm text-muted'">Дата</dt>
                  <dd :class="isKiosk ? 'text-xl font-medium text-highlighted text-right' : 'text-sm font-medium text-highlighted text-right'">
                    {{ formatDateRu(event.date) }}
                  </dd>
                </div>

                <div class="flex items-start justify-between gap-3">
                  <dt :class="isKiosk ? 'text-xl text-muted' : 'text-sm text-muted'">Время</dt>
                  <dd :class="isKiosk ? 'text-xl font-medium text-highlighted text-right' : 'text-sm font-medium text-highlighted text-right'">
                    {{ timeLabel || 'Уточняется' }}
                  </dd>
                </div>

                <div class="flex items-start justify-between gap-3">
                  <dt :class="isKiosk ? 'text-xl text-muted' : 'text-sm text-muted'">Место</dt>
                  <dd :class="isKiosk ? 'text-xl font-medium text-highlighted text-right' : 'text-sm font-medium text-highlighted text-right'">
                    {{ event.schedule.location || 'Уточняется' }}
                  </dd>
                </div>

                <div class="flex items-start justify-between gap-3">
                  <dt :class="isKiosk ? 'text-xl text-muted' : 'text-sm text-muted'">Участники</dt>
                  <dd :class="isKiosk ? 'text-xl font-medium text-highlighted text-right' : 'text-sm font-medium text-highlighted text-right'">
                    {{ seatsLabel }}
                    <span v-if="isFull" class="block text-warning font-normal">мест больше нет</span>
                  </dd>
                </div>

                <div class="flex items-start justify-between gap-3">
                  <dt :class="isKiosk ? 'text-xl text-muted' : 'text-sm text-muted'">Статус</dt>
                  <dd :class="isKiosk ? 'text-xl font-medium text-highlighted text-right' : 'text-sm font-medium text-highlighted text-right'">
                    {{
                      isKiosk
                        ? (isArchivedEvent ? 'Архив (запись закрыта)' : (event.badge || 'Активное'))
                        : (isArchivedEvent ? 'Архив (запись закрыта)' : (isJoined ? 'Вы записаны' : 'Не записаны'))
                    }}
                  </dd>
                </div>
              </dl>

              <div v-if="!isKiosk && event.link && event.link !== '#'" class="pt-4">
                <UButton as="a" :href="event.link" target="_blank" rel="noopener noreferrer" size="lg" color="neutral" variant="solid" class="w-full justify-center" icon="i-lucide-external-link">
                  Открыть страницу
                </UButton>
              </div>
            </UCard>

            <!-- Записавшиеся — только организаторам (ADR-054) -->
            <UCard v-if="showParticipants" class="rounded-lg">
              <template #header>
                <div class="flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <UIcon name="i-lucide-users" class="size-5 text-primary" aria-hidden="true" />
                    <span class="text-sm font-semibold text-highlighted">Записавшиеся</span>
                  </div>
                  <span class="text-sm text-muted tabular-nums">{{ seatsLabel }}</span>
                </div>
              </template>

              <div v-if="participantsLoading && !participants.length" class="flex flex-col gap-3">
                <div v-for="i in 3" :key="i" class="flex items-center gap-3">
                  <USkeleton class="size-8 rounded-full" />
                  <USkeleton class="h-4 w-40" />
                </div>
              </div>
              <div v-else-if="participantsError" class="flex flex-col items-start gap-2">
                <p class="text-sm text-error">{{ participantsError }}</p>
                <UButton size="xs" color="neutral" variant="outline" label="Повторить" @click="loadParticipants" />
              </div>
              <UEmpty
                v-else-if="!participants.length"
                variant="naked"
                icon="i-lucide-user-round-x"
                title="Пока никто не записался"
                description="Сотрудники появятся здесь, как только нажмут «Записаться»."
              />
              <ol v-else class="flex flex-col gap-1 max-h-96 overflow-y-auto">
                <li v-for="(p, i) in participants" :key="p.id">
                  <RouterLink
                    :to="`/profile/${p.id}`"
                    class="flex items-center gap-3 rounded-md px-2 py-1.5 transition-colors hover:bg-elevated"
                  >
                    <span class="w-5 shrink-0 text-right text-xs text-dimmed tabular-nums">{{ i + 1 }}</span>
                    <UAvatar :src="userAvatarSrc(p)" :alt="p.name" size="sm" class="shrink-0" />
                    <span class="min-w-0 flex-1">
                      <span class="block truncate text-sm text-highlighted">{{ p.name }}</span>
                      <span class="block text-xs text-muted">{{ formatJoinedAt(p.joinedAt) }}</span>
                    </span>
                  </RouterLink>
                </li>
              </ol>
            </UCard>
          </div>
        </div>
      </div>

      <!-- Не найдено -->
      <div v-else class="py-10">
        <UEmpty
          icon="i-lucide-calendar-x"
          title="Мероприятие не найдено"
          description="Возможно, оно было удалено или вы перешли по неверной ссылке."
        />
      </div>
    </div>

    <!-- Slideover редактирования -->
    <USlideover
      v-model:open="editOpen"
      side="right"
      title="Редактирование мероприятия"
      description="Измените данные мероприятия"
    >
      <template #body>
        <div class="flex flex-col gap-4 py-2">
          <UForm :state="editState" class="space-y-4" @submit.prevent="handleEditSubmit">
            <UFormField label="Название" name="title" required>
              <UInput v-model="editState.title" size="lg" placeholder="Название мероприятия" class="w-full" />
            </UFormField>

            <UFormField label="Категория (бейдж)" name="badge">
              <USelect
                v-model="editState.badge"
                :content="slideoverSelectContent"
                :items="editBadgeOptions"
                placeholder="Выберите: Новое или Архив"
                size="lg"
                class="w-full"
              />
            </UFormField>

            <UFormField label="Описание" name="description">
              <UTextarea v-model="editState.description" size="lg" :rows="3" placeholder="Описание мероприятия" class="w-full" />
            </UFormField>

            <UFormField label="Дата проведения" name="date" required>
              <UInputDate v-model="editDateValue" size="lg" class="w-full">
                <template #trailing>
                  <UTooltip text="Выбрать дату">
                    <span class="inline-flex">
                      <UPopover :content="slideoverPopoverContent">
                        <UButton color="neutral" variant="link" size="sm" icon="i-lucide-calendar" aria-label="Выбрать дату" class="px-0" />
                        <template #content>
                          <UCalendar v-model="editDateValue" class="p-2" />
                        </template>
                      </UPopover>
                    </span>
                  </UTooltip>
                </template>
              </UInputDate>
            </UFormField>

            <EventScheduleFields
              v-model:time-start="editState.timeStart"
              v-model:time-end="editState.timeEnd"
              v-model:location="editState.location"
              v-model:max-participants="editState.maxParticipants"
              size="lg"
            />

            <UFormField label="Альбом фотогалереи" name="albumId">
              <USelect
                v-model="editState.albumId"
                :content="slideoverSelectContent"
                :items="albumSelectItems"
                placeholder="Выберите альбом"
                size="lg"
                class="w-full"
              />
            </UFormField>

            <UFormField label="Изображение" name="image">
              <div class="flex flex-col gap-2 w-full">
                <input id="editFileInput" type="file" accept="image/jpeg,image/png,image/webp" class="hidden" @change="onEditImageSelected" />
                <img v-if="editImagePreview" :src="editImagePreview" alt="Предпросмотр" class="w-full h-40 object-cover rounded-lg" />
                <label for="editFileInput" class="flex items-center justify-center gap-2 cursor-pointer rounded-lg border border-default px-4 py-2.5 text-sm font-medium text-default hover:bg-elevated/50 transition-colors">
                  <UIcon name="i-lucide-upload" class="text-base shrink-0" />
                  {{ editImageFile ? 'Изменить фото' : 'Заменить фото' }}
                </label>
                <p v-if="editImageFile" class="text-xs text-muted truncate px-1">{{ editImageFile.name }}</p>
              </div>
            </UFormField>

            <UAlert v-if="editError" color="error" variant="subtle" icon="i-lucide-alert-circle" :description="editError" />
          </UForm>
        </div>
      </template>
      <template #footer>
        <div class="flex justify-between gap-3 items-center w-full">
          <UButton color="neutral" variant="outline" size="xl" class="w-full justify-center" @click="editOpen = false">Отмена</UButton>
          <UButton size="xl" class="w-full justify-center" :loading="editSubmitting" @click="handleEditSubmit">Сохранить</UButton>
        </div>
      </template>
    </USlideover>

    <!-- Модал подтверждения удаления -->
    <UModal v-model:open="deleteConfirmOpen" title="Удалить мероприятие?" description="Подтвердите удаление мероприятия">
      <template #body>
        <p class="text-default">
          Вы уверены, что хотите удалить <strong>«{{ event?.title }}»</strong>? Это действие нельзя отменить.
        </p>
        <UAlert v-if="deleteError" color="error" variant="subtle" icon="i-lucide-alert-circle" :description="deleteError" class="mt-3" />
      </template>
      <template #footer>
        <div class="flex gap-3 justify-end w-full">
          <UButton color="neutral" variant="outline" size="lg" @click="deleteConfirmOpen = false">Отмена</UButton>
          <UButton color="error" size="lg" :loading="deleteSubmitting" @click="handleDelete">Удалить</UButton>
        </div>
      </template>
    </UModal>
  </UMain>
</template>

<style>
/* USelect / UPopover / USelectMenu поверх USlideover */
:global([data-reka-select-content]),
:global([data-reka-popover-content]),
:global([data-reka-combobox-content]) {
  z-index: 100 !important;
}
</style>