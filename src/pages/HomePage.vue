<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted, nextTick } from 'vue';
import type { TabsItem } from '@nuxt/ui';
import { onBeforeRouteLeave } from 'vue-router';
import { resolveNewsImageSrc } from '../composables/useNewsData';
import { useNewsFeed, useFeedSentinel } from '../composables/useNewsFeed';
import { useBirthdayColleagues } from '../composables/useBirthdayColleagues';
import { useBirthdayGreetings } from '../composables/useBirthdayGreetings';
import BirthdayGreetingSlideover from '../components/BirthdayGreetingSlideover.vue';
import { attachAbsenceStorageSync, hasActiveAbsence } from '../stores/absenceJournal';
import { useSectionAccess } from '../composables/useSectionAccess';
import { apiSessionUpload } from '../composables/useAuthSession';
import { useHeaderUser } from '../composables/useHeaderUser';
import LearningHomeWidget from './Courses/components/LearningHomeWidget.vue';
import HomeNewsCard from '../components/home/HomeNewsCard.vue';
import { DEVBLOG_AUTHOR, isDevblogCategory } from '../composables/useDevblog';
import HomeCalendarWidget from '../components/home/HomeCalendarWidget.vue';
import HomeEventsWidget from '../components/home/HomeEventsWidget.vue';
import HomeAbsenceWidget from '../components/home/HomeAbsenceWidget.vue';
import { usePortalServices } from '../composables/usePortalServices';


const { canEditSection, ensureLoaded: ensureSectionAccess } = useSectionAccess();
ensureSectionAccess();
const canEditBirthdays = computed(() => canEditSection('birthdays'));

const learningAsideVisible = ref(false);
const calendarAsideVisible = ref(false);
const eventsAsideVisible = ref(false);

const { enabledServices, ensureLoaded: ensurePortalServices } = usePortalServices();
ensurePortalServices();

const { profile, headerName } = useHeaderUser();

const greetingTitle = computed(() => {
  const hour = new Date().getHours();
  let prefix = 'Добрый день';
  if (hour < 6) prefix = 'Доброй ночи';
  else if (hour < 12) prefix = 'Доброе утро';
  else if (hour >= 18) prefix = 'Добрый вечер';

  const p = profile.value;
  const namePatronymic = [p?.firstname, p?.lastname].filter(Boolean).join(' ').trim();
  const name = namePatronymic || headerName.value || 'коллега';
  return `${prefix}, ${name}!`;
});

type HomeService = {
  id: string;
  label: string;
  icon: string;
  to: string;
  external?: boolean;
};

const HOME_SERVICES_LIMIT = 4;

const homeServices = computed<HomeService[]>(() => {
  const fromApi: HomeService[] = enabledServices.value
    .slice(0, HOME_SERVICES_LIMIT)
    .map((s) => {
      if (s.kind === 'external' && s.externalUrl) {
        return {
          id: `svc-${s.id}`,
          label: s.label,
          icon: s.icon,
          to: s.externalUrl,
          external: true,
        };
      }
      return {
        id: `svc-${s.id}`,
        label: s.label,
        icon: s.icon,
        to: s.path || '/services',
      };
    });
  return [
    ...fromApi,
    {
      id: 'all',
      label: 'Все сервисы',
      icon: 'i-lucide-layout-grid',
      to: '/services',
    },
  ];
});

// Плиток бывает от 1 до 5 (до 4 сервисов + «Все сервисы») — колонок столько же,
// чтобы ряд занимал всю ширину. Классы полные: Tailwind не видит собранные строки.
const SERVICES_GRID_COLS: Record<number, string> = {
  1: 'sm:grid-cols-1 lg:grid-cols-1',
  2: 'sm:grid-cols-2 lg:grid-cols-2',
  3: 'sm:grid-cols-3 lg:grid-cols-3',
  4: 'sm:grid-cols-2 lg:grid-cols-4',
  5: 'sm:grid-cols-3 lg:grid-cols-5',
};
const servicesGridClass = computed(
  () => SERVICES_GRID_COLS[homeServices.value.length] ?? 'sm:grid-cols-3 lg:grid-cols-5',
);

type NewsFeedItem = {
  id: string;
  views: number;
  title: string;
  description: string;
  imageSrc: string;
  to: string;
  date?: string;
  createdAt?: string | null;
  category: string;
};

const homeScrollEl = ref<HTMLElement | null>(null);
const newsSentinelEl = ref<HTMLElement | null>(null);

const {
  items: feedRecords,
  hasMore: feedHasMore,
  loading: feedLoading,
  initialLoading: feedInitialLoading,
  error: feedError,
  scrollTop: savedScrollTop,
  activeTab: newsTab,
  loadInitial,
  loadMore,
  refresh,
  saveScrollTop,
  resolveLikesViews,
} = useNewsFeed();

function newsPreviewText(html: string, maxLen: number): string {
  const plain = String(html ?? '')
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  return plain.length > maxLen ? `${plain.slice(0, maxLen)}…` : plain;
}

const DEPARTMENT_TABS = [
  {
    value: 'municipal',
    label: 'Отдел муниципальной службы',
  },
  {
    value: 'hr',
    label: 'Отдел кадров',
  },
  {
    value: 'motivation',
    label: 'Отдел развития и мотивации персонала',
  },
] as const;

type DepartmentTabValue = (typeof DEPARTMENT_TABS)[number]['value'];

function isDepartmentTab(tab: string): tab is DepartmentTabValue {
  return DEPARTMENT_TABS.some((d) => d.value === tab);
}

function departmentCategoryFilter(tab: string): string | null {
  const found = DEPARTMENT_TABS.find((d) => d.value === tab);
  return found?.label ?? null;
}

const newsItems = computed<NewsFeedItem[]>(() =>
  feedRecords.value.map((n) => {
    const imageSrc = resolveNewsImageSrc(n.imagePath);
    const counts = resolveLikesViews(n.id);
    return {
      id: n.id,
      views: counts.views,
      title: n.title || `Новость #${n.id}`,
      description: newsPreviewText(n.description, 220),
      imageSrc: imageSrc || '/src/img/Logo.svg',
      to: `/news/${n.id}`,
      date: n.date || undefined,
      createdAt: n.createdAt,
      category: n.category || 'Новости',
    };
  }),
);

const newsTabItems = computed<TabsItem[]>(() => [
  { label: 'Лента новостей', value: 'feed', class: 'shrink-0' },
  ...DEPARTMENT_TABS.map((d) => ({
    label: d.label,
    value: d.value,
    class: 'min-w-0',
    ui: { label: 'truncate' },
  })),
]);

const displayNewsItems = computed(() => newsItems.value);

const feedSentinelEnabled = computed(() => {
  if (feedLoading.value || feedError.value || !feedHasMore.value) return false;
  return true;
});

useFeedSentinel({
  root: homeScrollEl,
  sentinel: newsSentinelEl,
  enabled: feedSentinelEnabled,
  onIntersect: () => {
    void loadMore();
  },
  rootMargin: '600px 0px',
});

if (newsTab.value === 'ofo') {
  newsTab.value = 'feed';
}

watch(newsTab, (tab) => {
  if (isDepartmentTab(tab)) {
    const cat = departmentCategoryFilter(tab);
    if (!cat) return;
    void loadInitial({ category: cat, force: true });
    return;
  }
  void loadInitial({ category: null });
});

function onHomeScroll() {
  const el = homeScrollEl.value;
  if (!el) return;
  saveScrollTop(el.scrollTop);
}

async function restoreHomeScroll() {
  await nextTick();
  requestAnimationFrame(() => {
    const el = homeScrollEl.value;
    if (!el) return;
    const y = savedScrollTop.value;
    if (y > 0) el.scrollTop = y;
  });
}

onBeforeRouteLeave(() => {
  const el = homeScrollEl.value;
  if (el) saveScrollTop(el.scrollTop);
});

function retryFeed() {
  if (feedRecords.value.length > 0 && feedHasMore.value) {
    void loadMore();
    return;
  }
  void refresh({
    category: isDepartmentTab(newsTab.value)
      ? departmentCategoryFilter(newsTab.value)
      : null,
  });
}

const {
  birthdayGroups,
  loading: birthdaysLoading,
  error: birthdaysError,
  ensureLoaded: ensureBirthdaysLoaded,
  reload: reloadBirthdays,
} = useBirthdayColleagues();
ensureBirthdaysLoaded();

const visibleBirthdayGroups = computed(() =>
  birthdayGroups.value.filter((g) => g.people.length > 0),
);

const showBirthdaysWidget = computed(
  () => visibleBirthdayGroups.value.length > 0 || canEditBirthdays.value,
);

const hasAsideContent = computed(
  () =>
    hasActiveAbsence.value ||
    learningAsideVisible.value ||
    calendarAsideVisible.value ||
    eventsAsideVisible.value ||
    showBirthdaysWidget.value,
);

const { ensureLoaded: ensureGreetingsLoaded, greetState, openGreeting } = useBirthdayGreetings();
ensureGreetingsLoaded();

// ── Админ: загрузка дат рождений из xlsx ──────────────────────────────────────
const MONTH_NAMES = [
  'январь',
  'февраль',
  'март',
  'апрель',
  'май',
  'июнь',
  'июль',
  'август',
  'сентябрь',
  'октябрь',
  'ноябрь',
  'декабрь',
];
const birthdayUploadOpen = ref(false);
const birthdayFile = ref<File | File[] | undefined>(undefined);
const birthdayUploading = ref(false);
const birthdayUploadError = ref<string | null>(null);
const birthdayManifest = ref<Record<string, { filename?: string; year?: number }>>({});

const birthdayMonths = computed(() =>
  MONTH_NAMES.map((name, i) => ({
    month: i + 1,
    name,
    filename: birthdayManifest.value[String(i + 1)]?.filename ?? null,
  })),
);

async function loadBirthdayManifest() {
  try {
    const res = await fetch('/api/birthdays.php?manifest=1');
    const json = await res.json();
    if (json.success) birthdayManifest.value = json.data ?? {};
  } catch {
    /* молча */
  }
}

function openBirthdayUpload() {
  birthdayUploadError.value = null;
  birthdayFile.value = undefined;
  birthdayUploadOpen.value = true;
  void loadBirthdayManifest();
}

watch(birthdayFile, async (val) => {
  const file = Array.isArray(val) ? val[0] : val;
  if (!file) return;
  birthdayUploading.value = true;
  birthdayUploadError.value = null;
  try {
    const fd = new FormData();
    fd.append('file', file);
    const json = await apiSessionUpload('/api/birthdays.php', fd);
    if (!json.success) throw new Error(json.message || 'Ошибка загрузки');
    birthdayManifest.value = (json.data as any) ?? birthdayManifest.value;
    await reloadBirthdays();
  } catch (e: any) {
    birthdayUploadError.value = e?.message ?? 'Ошибка загрузки';
  } finally {
    birthdayUploading.value = false;
    birthdayFile.value = undefined;
  }
});

onMounted(() => {
  attachAbsenceStorageSync();

  const cat = isDepartmentTab(newsTab.value)
    ? departmentCategoryFilter(newsTab.value)
    : null;
  void loadInitial({ category: cat }).then(() => restoreHomeScroll());

  const el = homeScrollEl.value;
  if (el) el.addEventListener('scroll', onHomeScroll, { passive: true });
});

onUnmounted(() => {
  const el = homeScrollEl.value;
  if (el) {
    saveScrollTop(el.scrollTop);
    el.removeEventListener('scroll', onHomeScroll);
  }
});
</script>

<template>
  <UMain class="flex flex-1 flex-col w-full min-w-0 h-full min-h-0 max-h-full overflow-hidden">
    <div
      ref="homeScrollEl"
      class="flex w-full flex-1 flex-col gap-2 min-h-0 overflow-y-auto scrollbar-hide"
      :class="hasAsideContent ? '' : 'items-center'"
    >
      <div
        class="flex w-full min-w-0 flex-col gap-2"
        :class="hasAsideContent ? '' : 'max-w-4xl'"
      >
      <UPageHeader
        :title="greetingTitle"
        class="border-none py-4 px-0 w-full"
        :ui="{
          root: 'w-full',
          container: 'w-full mx-0',
          wrapper: 'w-full',
          title: 'text-2xl font-bold leading-8 text-highlighted',
        }"
      />

      <div
        class="w-full min-w-0 gap-4 items-start"
        :class="
          hasAsideContent
            ? 'grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_420px]'
            : 'flex flex-col'
        "
      >
        <div class="flex min-w-0 w-full flex-col gap-4">
      <section class="flex flex-col gap-4 w-full" aria-labelledby="home-services-title">
        <div class="flex items-center gap-1 min-w-0">
          <h2 id="home-services-title" class="text-lg font-bold leading-7 text-highlighted">
            Сервисы
          </h2>
          <UTooltip text="Быстрый доступ к корпоративным сервисам">
            <UButton
              type="button"
              color="neutral"
              variant="ghost"
              size="xs"
              icon="i-lucide-info"
              square
              aria-label="О сервисах"
            />
          </UTooltip>
        </div>

        <div class="grid w-full grid-cols-2 gap-3" :class="servicesGridClass">
          <UPageCard
            v-for="svc in homeServices"
            :key="svc.id"
            :title="svc.label"
            :icon="svc.icon"
            :to="svc.to"
            :target="svc.external ? '_blank' : undefined"
            :rel="svc.external ? 'noopener noreferrer' : undefined"
            :external="svc.external || undefined"
            variant="soft"
            class="bg-elevated"
          />
        </div>
      </section>

        <!-- Центр: лента -->
        <section class="flex flex-col gap-4 min-w-0 w-full" aria-label="Лента новостей">
          <UTabs
            v-model="newsTab"
            :items="newsTabItems"
            variant="link"
            color="primary"
            size="md"
            :content="false"
            class="w-full max-w-full border-b border-default"
            :ui="{
              root: 'w-full max-w-full items-stretch',
              list: 'w-full h-12 gap-1.5',
              trigger: 'h-12 min-w-0 justify-start focus-visible:outline-none focus-visible:ring-0',
              label: 'truncate',
            }"
          />

          <div class="flex w-full min-w-0 flex-col gap-4">
            <template v-if="feedInitialLoading && !displayNewsItems.length">
              <USkeleton
                v-for="n in 3"
                :key="`sk-init-${n}`"
                class="h-[300px] w-full rounded-panel"
              />
            </template>

            <template v-else-if="displayNewsItems.length">
              <HomeNewsCard
                v-for="item in displayNewsItems"
                :key="item.id"
                :id="item.id"
                :title="item.title"
                :description="item.description"
                :image-src="item.imageSrc"
                :image-alt="item.title"
                :to="item.to"
                :date="item.date"
                :created-at="item.createdAt"
                :views="item.views"
                :author-name="isDevblogCategory(item.category) ? DEVBLOG_AUTHOR : undefined"
                :author-role="item.category"
              />

              <template v-if="feedLoading && !feedInitialLoading">
                <USkeleton
                  v-for="n in 2"
                  :key="`sk-more-${n}`"
                  class="h-[300px] w-full rounded-panel"
                />
              </template>

              <div
                v-if="feedError && !feedInitialLoading"
                class="flex flex-col items-center gap-3 py-4"
              >
                <p class="text-sm text-error">Не удалось загрузить новости</p>
                <UButton
                  type="button"
                  color="neutral"
                  variant="outline"
                  size="md"
                  icon="i-lucide-refresh-cw"
                  @click="retryFeed"
                >
                  Повторить
                </UButton>
              </div>

              <div
                ref="newsSentinelEl"
                class="h-1 w-full shrink-0"
                aria-hidden="true"
              />
            </template>

            <div
              v-else-if="feedError"
              class="flex flex-col items-center gap-3 py-10"
            >
              <p class="text-sm text-error">Не удалось загрузить новости</p>
              <UButton
                type="button"
                color="neutral"
                variant="outline"
                size="md"
                icon="i-lucide-refresh-cw"
                @click="retryFeed"
              >
                Повторить
              </UButton>
            </div>

            <UEmpty
              v-else-if="isDepartmentTab(newsTab)"
              variant="naked"
              icon="i-lucide-building-2"
              title="Нет новостей отдела"
              description="Вкладка покажет материалы, если категория новости совпадёт с названием отдела."
              class="w-full py-10"
            />
            <UEmpty
              v-else
              variant="naked"
              icon="i-lucide-newspaper"
              title="Новостей пока нет"
              description="Как только появятся публикации, они отобразятся здесь."
              class="w-full py-10"
            />
          </div>
        </section>
        </div>

        <!-- Правая колонка -->
        <aside
          v-show="hasAsideContent"
          class="w-full min-w-0 flex flex-col gap-4"
        >
          <HomeAbsenceWidget />

          <LearningHomeWidget @visible="learningAsideVisible = $event" />
          <HomeCalendarWidget @visible="calendarAsideVisible = $event" />
          <HomeEventsWidget @visible="eventsAsideVisible = $event" />

          <UCard
            v-if="showBirthdaysWidget"
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
                  <h2 class="text-lg font-bold leading-7 text-highlighted truncate">Дни рождения коллег</h2>
                  <UTooltip text="Именинники на сегодня и ближайшие дни">
                    <UButton
                      type="button"
                      color="neutral"
                      variant="ghost"
                      size="xs"
                      icon="i-lucide-info"
                      square
                      aria-label="О днях рождения"
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
            <UButton
              v-if="canEditBirthdays"
              label="Загрузить даты xlsx"
              icon="i-lucide-upload"
              color="neutral"
              variant="outline"
              size="sm"
              block
              @click="openBirthdayUpload"
            />
            <div v-if="birthdaysLoading" class="flex flex-col gap-2">
              <USkeleton v-for="n in 3" :key="n" class="h-12 w-full rounded-lg" />
            </div>
            <p v-else-if="birthdaysError" class="text-sm text-error">{{ birthdaysError }}</p>
            <p v-else-if="!visibleBirthdayGroups.length" class="text-sm text-muted">
              В ближайшие дни именинников нет
            </p>
            <div
              v-for="group in visibleBirthdayGroups"
              :key="group.id"
              class="flex flex-col gap-2"
            >
              <p class="text-xs font-medium leading-4 text-muted">
                {{ group.dayLabel }}
              </p>
              <div
                v-for="person in group.people"
                :key="person.id"
                class="flex items-center gap-2"
              >
                <UUser
                  :name="person.name"
                  :description="person.role || undefined"
                  :avatar="{ src: person.avatar, alt: person.name }"
                  size="md"
                  class="min-w-0 flex-1"
                />
                <UTooltip v-if="greetState(person.userId, group.date) === 'greet'" text="Поздравить">
                  <UButton
                    type="button"
                    color="neutral"
                    variant="outline"
                    size="sm"
                    icon="i-lucide-gift"
                    square
                    :aria-label="`Поздравить: ${person.name}`"
                    @click="openGreeting({ userId: person.userId!, name: person.name, avatar: person.avatar }, group.date)"
                  />
                </UTooltip>
                <UTooltip v-else-if="greetState(person.userId, group.date) === 'greeted'" text="Вы поздравили — открыть стену">
                  <UButton
                    :to="`/profile/${person.userId}`"
                    color="success"
                    variant="soft"
                    size="sm"
                    icon="i-lucide-check"
                    square
                    :aria-label="`Вы поздравили: ${person.name}. Открыть стену`"
                  />
                </UTooltip>
              </div>
            </div>
          </UCard>

        </aside>
      </div>
      </div>
    </div>

    <USlideover
      v-model:open="birthdayUploadOpen"
      side="right"
      title="Загрузка дат рождений (xlsx)"
      description=""
    >
      <template #body>
        <div class="space-y-4">
          <UFileUpload
            v-model="birthdayFile"
            accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            label="Перетащите xlsx сюда"
            description="Один файл = один месяц. A1 — месяц, B1 — год, далее ФИО и дата."
            class="w-full min-h-32"
          />
          <UAlert
            v-if="birthdayUploadError"
            color="error"
            variant="subtle"
            icon="i-lucide-alert-circle"
            :description="birthdayUploadError"
          />
          <p v-if="birthdayUploading" class="text-sm text-muted">Загрузка…</p>

          <div class="flex flex-col divide-y divide-default rounded-lg ring ring-default">
            <div
              v-for="m in birthdayMonths"
              :key="m.month"
              class="flex items-center justify-between gap-3 px-3 py-2"
            >
              <span class="text-sm capitalize">{{ m.name }}</span>
              <span
                class="text-sm truncate max-w-[60%]"
                :class="m.filename ? 'text-default' : 'text-muted italic'"
                :title="m.filename || 'не загружено'"
              >
                {{ m.filename || 'не загружено' }}
              </span>
            </div>
          </div>
        </div>
      </template>
    </USlideover>

    <BirthdayGreetingSlideover />

    <ScrollToTopButton :target="homeScrollEl" />
  </UMain>
</template>
