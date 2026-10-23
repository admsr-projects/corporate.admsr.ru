<script setup lang="ts">
import { computed, reactive, ref, watch, onMounted, onUnmounted } from 'vue';
import { useRoute } from 'vue-router';
import { useNewsData, formatNewsDate, resolveNewsImageSrc, mapApiRow } from '../../composables/useNewsData';
import { DEVBLOG_CATEGORY } from '../../composables/useDevblog';
import { newsEditorToolbarItems } from '../../composables/newsEditorToolbar';
import { newsEditorExtensions, newsEditorEmojiMenuItems } from '../../composables/newsEditorExtensions';
import { newsEditorHandlers } from '../../composables/newsEditorHandlers';
import { newsEditorSlideoverUi } from '../../composables/newsEditorSlideoverUi';
import { useAppToast } from '../../composables/useAppToast';
import { useSectionAccess } from '../../composables/useSectionAccess';
import { apiSessionFetch } from '../../composables/useAuthSession';
import { useCursorFeed } from '../../composables/useCursorFeed';
import { useFeedSentinel } from '../../composables/useFeedSentinel';

type NewsCard = {
  id: string;
  rawDate: string;
  title: string;
  date: string;
  to: string;
  views: number;
  coverSrc: string;
};

const NEWS_PAGE_LIMIT = 12;

const newsPlaceholder = (() => {
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="450" viewBox="0 0 800 450">` +
    `<rect width="800" height="450" fill="#1e293b"/>` +
    `<text x="50%" y="50%" dominant-baseline="middle" text-anchor="middle" ` +
    `fill="#94a3b8" font-family="sans-serif" font-size="72" font-weight="700" letter-spacing="10">НОВОСТЬ</text>` +
    `</svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
})();

function viewsLabel(n: number): string {
  const v = Math.max(0, Math.round(Number(n) || 0));
  const mod10 = v % 10;
  const mod100 = v % 100;
  if (mod10 === 1 && mod100 !== 11) return `${v.toLocaleString('ru-RU')} просмотр`;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) {
    return `${v.toLocaleString('ru-RU')} просмотра`;
  }
  return `${v.toLocaleString('ru-RU')} просмотров`;
}

const route = useRoute();
const isKiosk = computed(() => route.matched?.some((r) => r.meta?.kiosk));
const newsDetailPrefix = computed(() => (isKiosk.value ? '/kiosk/news/' : '/news/'));

const { upsertItems, reload: reloadNewsCache } = useNewsData();

const searchQuery = ref('');
const searchForApi = ref('');
let searchTimer: ReturnType<typeof setTimeout> | null = null;
watch(searchQuery, (q) => {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    searchForApi.value = q.trim();
  }, 300);
});

const sortKey = ref<'newest' | 'oldest' | 'title-asc' | 'title-desc'>('newest');
const sortOptions = [
  { value: 'newest', label: 'Сначала новые' },
  { value: 'oldest', label: 'Сначала старые' },
  { value: 'title-asc', label: 'По названию (А‑Я)' },
  { value: 'title-desc', label: 'По названию (Я‑А)' },
];

const {
  items: feedItems,
  loading: feedLoading,
  initialLoading,
  error,
  loadInitial,
  loadMore,
  refresh,
  sentinelEnabled,
} = useCursorFeed<NewsCard>({
  buildUrl: (cursor) => {
    const params = new URLSearchParams();
    params.set('limit', String(NEWS_PAGE_LIMIT));
    if (cursor) params.set('cursor', cursor);
    if (searchForApi.value) params.set('search', searchForApi.value);
    // Девблоги — только в ленте рабочего стола, во вкладке «Новости» их нет.
    params.set('excludeCategory', DEVBLOG_CATEGORY);
    return `/api/news.php?${params.toString()}`;
  },
  mapItem: (raw) => {
    const n = mapApiRow(raw as Record<string, unknown>);
    if (!n) return null;
    upsertItems([n]);
    const imageSrc = resolveNewsImageSrc(n.imagePath);
    return {
      id: n.id,
      rawDate: n.date,
      title: n.title || `Новость #${n.id}`,
      date: formatNewsDate(n.date),
      to: `${newsDetailPrefix.value}${n.id}`,
      views: n.views ?? 0,
      coverSrc: imageSrc || newsPlaceholder,
    };
  },
  getId: (item) => item.id,
});

const loading = computed(() => initialLoading.value);

watch(searchForApi, () => {
  void loadInitial(true);
});

const { toast } = useAppToast();
watch(
  error,
  (val) => {
    if (!val) return;
    toast.add({
      title: 'Не удалось загрузить новости',
      description: String(val),
      color: 'error',
      icon: 'i-lucide-alert-circle',
    });
  },
  { immediate: true },
);

const { canEditSection, ensureLoaded: ensureSectionAccess } = useSectionAccess();
ensureSectionAccess();

const appendEditorEmojiTo = () => document.body;

const headerLinks = computed(() => {
  if (!canEditSection('news') || isKiosk.value) return [];
  return [
    {
      label: 'Добавить новость',
      color: 'neutral',
      variant: 'outline',
      size: 'md',
      onClick: openCreate,
    },
  ];
});

const filtered = computed(() => {
  const items = feedItems.value.slice();
  items.sort((a, b) => {
    if (sortKey.value === 'newest') return String(b.rawDate ?? '').localeCompare(String(a.rawDate ?? ''));
    if (sortKey.value === 'oldest') return String(a.rawDate ?? '').localeCompare(String(b.rawDate ?? ''));
    if (sortKey.value === 'title-asc') return String(a.title ?? '').localeCompare(String(b.title ?? ''), 'ru-RU');
    return String(b.title ?? '').localeCompare(String(a.title ?? ''), 'ru-RU');
  });
  return items;
});

const mainScrollEl = ref<HTMLElement | null>(null);
const newsSentinelEl = ref<HTMLElement | null>(null);

useFeedSentinel({
  root: mainScrollEl,
  sentinel: newsSentinelEl,
  enabled: sentinelEnabled,
  onIntersect: () => {
    void loadMore();
  },
  rootMargin: '600px 0px',
});

const createBadgeOptions = [
  { value: 'Новости', label: 'Новости' },
  { value: 'Мероприятия', label: 'Мероприятия' },
  { value: 'Объявления', label: 'Объявления' },
  { value: 'Архив', label: 'Архив' },
];

const createOpen = ref(false);

type CreateFormState = {
  title: string;
  category: string | undefined;
  description: string;
  date: string;
};

const createState = reactive<CreateFormState>({
  title: '',
  category: undefined,
  description: '',
  date: '',
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
  createState.category = undefined;
  createState.description = '';
  createState.date = '';
  createImageFile.value = null;
  if (createImagePreview.value) URL.revokeObjectURL(createImagePreview.value);
  createImagePreview.value = '';
  createDateValue.value = null;
  createError.value = null;
}

function openCreate() {
  resetCreateForm();
  createOpen.value = true;
}

function validateCreate(): boolean {
  if (!createState.title.trim()) {
    createError.value = 'Заполните название новости.';
    return false;
  }
  if (!createState.date) {
    createError.value = 'Выберите дату.';
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
    let imagePath: string | null = null;

    if (createImageFile.value) {
      const formData = new FormData();
      formData.append('image', createImageFile.value);
      const uploadRes = await fetch('/api/Upload/upload.php', { method: 'POST', body: formData });
      const uploadJson = await uploadRes.json();
      if (!uploadJson.success) throw new Error(uploadJson.message || 'Ошибка загрузки изображения');
      imagePath = uploadJson.data.image as string;
    }

    const json = await apiSessionFetch('/api/news.php', {
      method: 'POST',
      json: {
        title: createState.title.trim(),
        category: createState.category || '',
        description: createState.description.trim(),
        date: createState.date,
        image_path: imagePath,
      },
    });
    if (!json.success) throw new Error(json.message || 'Ошибка создания');

    toast.add({ title: 'Новость создана', color: 'success', icon: 'i-lucide-check-circle' });
    createOpen.value = false;
    resetCreateForm();
    await refresh();
    void reloadNewsCache();
  } catch (e: any) {
    createError.value = e.message ?? 'Ошибка при создании новости';
  } finally {
    createSubmitting.value = false;
  }
}

onMounted(() => {
  void loadInitial();
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
      <div class="flex flex-col gap-6 w-full max-w-[1600px] mx-auto">
        <UPageHeader
          headline="Корпоративная жизнь"
          title="Новости"
          description="Свежие события и объявления корпоративного портала"
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
            placeholder="Найти новость..."
            class="w-full min-w-0 flex-1"
          />
          <USelect
            v-model="sortKey"
            :items="sortOptions"
            size="md"
            color="neutral"
            class="w-full sm:w-56 shrink-0"
          />
        </div>
        <div v-else class="flex justify-end">
          <USelect
            v-model="sortKey"
            :items="sortOptions"
            size="md"
            color="neutral"
            class="w-full sm:w-56 shrink-0"
          />
        </div>

        <div v-if="loading" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          <USkeleton v-for="i in 6" :key="i" class="h-64 rounded-panel" />
        </div>

        <UEmpty
          v-else-if="!filtered.length"
          variant="naked"
          icon="i-lucide-newspaper"
          title="Новости не найдены"
          description="Измените поиск или создайте новую запись."
          class="w-full py-10"
        />

        <div
          v-else
          class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3"
        >
          <RouterLink
            v-for="post in filtered"
            :key="post.id"
            :to="post.to"
            class="group flex flex-col gap-3 rounded-panel p-px transition ring-1 ring-transparent hover:ring-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          >
            <div class="relative aspect-[16/10] overflow-hidden rounded-panel bg-muted">
              <img
                :src="post.coverSrc"
                alt=""
                aria-hidden="true"
                loading="lazy"
                decoding="async"
                class="absolute inset-0 size-full scale-110 object-cover blur-2xl"
              />
              <img
                :src="post.coverSrc"
                :alt="post.title"
                loading="lazy"
                decoding="async"
                class="relative z-10 block size-full object-contain transition duration-300 group-hover:scale-[1.02]"
              />
              <span
                class="absolute bottom-2 right-2 z-20 inline-flex items-center gap-1.5 rounded-md bg-black/65 px-2 py-1 text-xs text-white tabular-nums backdrop-blur-sm"
              >
                <UIcon name="i-lucide-eye" class="size-3.5 shrink-0" />
                {{ viewsLabel(post.views) }}
              </span>
            </div>
            <div class="flex min-w-0 flex-col gap-1 px-2.5 pb-2.5">
              <p v-if="post.date" class="text-sm text-muted">
                {{ post.date }}
              </p>
              <h2 class="text-base font-semibold text-highlighted line-clamp-2 text-pretty">
                {{ post.title }}
              </h2>
            </div>
          </RouterLink>
        </div>

        <div
          v-if="!loading && feedLoading"
          class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3"
        >
          <USkeleton v-for="i in 3" :key="`more-${i}`" class="h-64 rounded-panel" />
        </div>

        <div
          v-if="filtered.length"
          ref="newsSentinelEl"
          class="h-1 w-full shrink-0"
          aria-hidden="true"
        />
      </div>
    </div>

    <ScrollToTopButton :target="mainScrollEl" />

    <USlideover
      v-model:open="createOpen"
      side="right"
      title="Новая новость"
      description="Заполните данные новости"
      :ui="{
        content: '!max-w-full sm:!max-w-2xl lg:!max-w-4xl xl:!max-w-5xl',
      }"
    >
      <template #body>
        <UForm :state="createState" class="space-y-4" @submit.prevent="handleCreateSubmit">
          <UFormField label="Название" name="title" required>
            <UInput
              v-model="createState.title"
              size="xl"
              class="w-full"
              placeholder="Введите название новости"
            />
          </UFormField>

          <UFormField label="Категория" name="category">
            <USelect
              v-model="createState.category"
              :items="createBadgeOptions"
              size="xl"
              class="w-full"
            />
          </UFormField>

          <UFormField label="Описание" name="description">
            <UEditor
              v-slot="{ editor }"
              v-model="createState.description"
              content-type="html"
              :extensions="newsEditorExtensions"
              :handlers="newsEditorHandlers"
              :ui="newsEditorSlideoverUi"
              placeholder="Текст новости…"
              class="w-full min-h-56 rounded-lg border border-default overflow-hidden"
            >
              <UEditorEmojiMenu
                :editor="editor"
                :items="newsEditorEmojiMenuItems"
                :append-to="appendEditorEmojiTo"
              />
              <UEditorToolbar
                :editor="editor"
                :items="newsEditorToolbarItems"
                class="sticky top-0 z-10 border-b border-default bg-default/95 backdrop-blur-sm px-2 py-1.5 overflow-x-auto"
              />
            </UEditor>
          </UFormField>

          <UFormField label="Дата" name="date" required>
            <UInputDate v-model="createDateValue" size="xl" class="w-full">
              <template #trailing>
                <UTooltip text="Выбрать дату">
                  <span class="inline-flex">
                    <UPopover>
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
            type="button"
            color="neutral"
            variant="outline"
            size="md"
            class="w-full justify-center"
            @click="createOpen = false"
          >
            Отмена
          </UButton>
          <UButton
            type="button"
            size="md"
            class="w-full justify-center"
            :loading="createSubmitting"
            @click="handleCreateSubmit"
          >
            Создать новость
          </UButton>
        </div>
      </template>
    </USlideover>
  </UMain>
</template>
