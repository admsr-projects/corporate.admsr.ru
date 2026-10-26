<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import type { DropdownMenuItem } from '@nuxt/ui';
import * as XLSX from 'xlsx';
import { useCoursesStore } from '../../../composables/useCoursesStore';
import { useAppToast } from '../../../composables/useAppToast';
import { useOfoTree } from '../../../composables/useOfoTree';
import { fmtDuration } from '../../../composables/useTestStats';
import OfoSelect from '../../../components/OfoSelect.vue';
import CourseStatusBadge from '../components/CourseStatusBadge.vue';
import { useAdminCoursePortalBreadcrumbs } from '../useAdminCoursePortalBreadcrumbs';
import { describeDeadline, formatDateTime, formatLastActivity } from '../courseDeadline';

const route = useRoute();
const store = useCoursesStore();
const { toast } = useAppToast();
useAdminCoursePortalBreadcrumbs();
const {
  ensureLoaded: ensureOfo,
  unitById,
  rootLabelOf,
} = useOfoTree();

const courseId = computed(() => Number(route.params.courseId));
const loading = ref(true);
/** Раньше ошибка загрузки давала пустой список и надпись «Пока нет участников». */
const loadError = ref<string | null>(null);
const summary = ref<Record<string, number | null>>({});
/** Сколько строк запрашиваем за раз; остальное — через поиск/фильтр или выгрузку. */
const PAGE_LIMIT = 200;
const rows = ref<any[]>([]);
const selectedId = ref<number | null>(
  route.query.enrollmentId ? Number(route.query.enrollmentId) : null,
);
const participant = ref<any | null>(null);
const detailLoading = ref(false);
const resetOpen = ref(false);
const resetTarget = ref<any | null>(null);
const resetting = ref(false);
const cancelOpen = ref(false);
const cancelTarget = ref<any | null>(null);
const cancelling = ref(false);
const exporting = ref(false);

const answersOpen = ref(false);
const answersTitle = ref('');
const answersLoading = ref(false);
const answersError = ref('');
const answersData = ref<any | null>(null);

const searchQuery = ref('');
/** '_all' = все ОФО; иначе id корневого подразделения (строка) */
const ofoFilter = ref<string>('_all');
/** '_all' = все статусы */
const statusFilter = ref<string>('_all');
let searchTimer: ReturnType<typeof setTimeout> | null = null;
let filtersReady = false;

const statusItems = [
  { label: 'Все статусы', value: '_all' },
  { label: 'Не начат', value: 'not_started' },
  { label: 'В процессе', value: 'in_progress' },
  { label: 'Завершён', value: 'completed' },
  { label: 'Не сдан', value: 'failed' },
  { label: 'Просрочен', value: 'overdue' },
  { label: 'Отменён', value: 'cancelled' },
];

/** Порядок строк: сервер отдаёт по дате назначения, остальное сортируем здесь. */
type SortKey = 'assigned' | 'fio' | 'progress' | 'deadline' | 'activity';
const sortKey = ref<SortKey>('assigned');
const sortItems: { label: string; value: SortKey }[] = [
  { label: 'Новые назначения', value: 'assigned' },
  { label: 'По ФИО', value: 'fio' },
  { label: 'Меньше прогресса', value: 'progress' },
  { label: 'Ближе срок', value: 'deadline' },
  { label: 'Давно не заходили', value: 'activity' },
];

function timeOr(iso: string | null | undefined, fallback: number) {
  const t = iso ? new Date(iso).getTime() : NaN;
  return Number.isNaN(t) ? fallback : t;
}

/** Отменённые — всегда в конце: они больше не проходят курс. */
const sortedRows = computed(() => {
  const cmp: Record<SortKey, (a: any, b: any) => number> = {
    assigned: () => 0,
    fio: (a, b) => String(a.fio).localeCompare(String(b.fio), 'ru'),
    progress: (a, b) => Number(a.progressPercent ?? 0) - Number(b.progressPercent ?? 0),
    deadline: (a, b) => timeOr(a.deadlineAt, Infinity) - timeOr(b.deadlineAt, Infinity),
    // Кто ни разу не заходил в курс, — выше всех.
    activity: (a, b) => timeOr(a.lastActivityAt, 0) - timeOr(b.lastActivityAt, 0),
  };
  return [...rows.value].sort((a, b) => {
    const cancelled = Number(a.status === 'cancelled') - Number(b.status === 'cancelled');
    return cancelled || cmp[sortKey.value](a, b);
  });
});

/** Балл имеет смысл только у курса с итоговым тестом — иначе везде был бы «—». */
const hasFinalTest = computed(() => {
  const v: any = store.version.value;
  return Boolean(v?.requireFinalTest || v?.finalTest);
});

const hasActiveFilters = computed(
  () => Boolean(searchQuery.value.trim()) || ofoFilter.value !== '_all' || statusFilter.value !== '_all',
);

/** Точное подразделение сотрудника (раньше показывали корневое ОФО). */
function displayOfoName(ofoId: number | null | undefined, fallback?: string | null) {
  const unit = ofoId != null ? unitById.value.get(Number(ofoId)) : null;
  if (unit) return unit.name;
  return fallback || null;
}

const detail = computed(() => {
  const p = participant.value;
  if (!p) return null;
  const enr = p.enrollment || {};
  const user = p.user || {};
  const selectedRow = rows.value.find((r) => r.id === selectedId.value);
  const ofoId = user.ofoId ?? selectedRow?.ofoId ?? null;
  const topics: any[] = Array.isArray(p.topics) ? p.topics : [];
  const materials: any[] = Array.isArray(p.materials) ? p.materials : [];
  const versionTopics: any[] = p.version?.topics || [];
  const topicSeconds = topics.reduce((sum, t) => sum + Number(t.activeSeconds || 0), 0);
  return {
    fio: user.fio || selectedRow?.fio || 'Сотрудник',
    role: String(user.role || '').trim(),
    email: String(user.email || '').trim(),
    phone: String(user.phone || '').trim(),
    ofoName: displayOfoName(ofoId, user.ofoName || selectedRow?.ofoName || null),
    status: enr.status || selectedRow?.status,
    progressPercent: enr.progress?.percent ?? selectedRow?.progressPercent ?? 0,
    topicsCompleted: Number(enr.progress?.topicsCompleted ?? 0),
    topicsTotal: Number(enr.progress?.topicsTotal ?? 0),
    finalScore: enr.finalScore ?? p.completion?.finalScore ?? selectedRow?.finalScore ?? null,
    dates: [
      { label: 'Назначен', value: formatDateTime(enr.assignedAt) },
      { label: 'Начал', value: formatDateTime(enr.startedAt) || 'ещё не начинал' },
      { label: 'Последняя активность', value: formatLastActivity(enr.lastActivityAt) || '—' },
      { label: 'Завершён', value: formatDateTime(enr.completedAt) },
    ].filter((d) => d.value),
    deadline: describeDeadline(enr.deadlineAt, ['completed', 'cancelled'].includes(String(enr.status))),
    totalSeconds: Number(p.completion?.totalActiveSeconds || 0) || topicSeconds,
    tests: Array.isArray(p.tests) ? p.tests : [],
    topics: topics.map((t) => {
      const all = versionTopics.find((vt) => Number(vt.id) === Number(t.topicId))?.materials || [];
      const done = materials.filter((m) => Number(m.topicId) === Number(t.topicId) && m.status === 'completed').length;
      return { ...t, materialsDone: done, materialsTotal: all.length };
    }),
  };
});

function scoreLabel(score: unknown) {
  const n = Number(score);
  return score == null || Number.isNaN(n) ? '' : `${Math.round(n)}%`;
}

function deadlineClass(tone?: string) {
  if (tone === 'error') return 'text-error';
  if (tone === 'warning') return 'text-warning';
  return 'text-muted';
}

function testKindLabel(t: any) {
  if (t?.type === 'final') return 'Итоговый';
  return t?.topicTitle ? `Тема: ${t.topicTitle}` : 'Тест темы';
}

function testResultLabel(t: any) {
  if (t?.passed === true) return 'Сдан';
  if (t?.passed === false) return 'Не сдан';
  // Попытки модуля тестов завершаются статусом 'completed' (V2__tests_module.sql).
  if (t?.status === 'completed' || t?.status === 'finished') return 'Завершён';
  if (t?.status === 'expired') return 'Время вышло';
  if (t?.status && t.status !== 'not_started') return 'В процессе';
  // Попыток не было — это «ещё не проходил», а не провал.
  return 'Не начат';
}

function testResultColor(t: any): 'success' | 'error' | 'warning' | 'neutral' {
  if (t?.passed === true) return 'success';
  if (t?.passed === false) return 'error';
  if (t?.status === 'completed' || t?.status === 'finished') return 'success';
  if (t?.status && t.status !== 'not_started') return 'warning';
  return 'neutral';
}

function rowMenuItems(row: any): DropdownMenuItem[][] {
  return [
    [
      {
        label: 'Открыть карточку',
        icon: 'i-lucide-user',
        onSelect() {
          void openDetail(row.id);
        },
      },
      {
        label: 'Обнулить результат',
        icon: 'i-lucide-rotate-ccw',
        color: 'error' as const,
        onSelect() {
          resetTarget.value = row;
          resetOpen.value = true;
        },
      },
      // Пройденный курс не отменяется (для повтора — «Обнулить»), отменённый — тем более.
      ...(row.status === 'completed' || row.status === 'cancelled'
        ? []
        : [
            {
              label: 'Отменить назначение',
              icon: 'i-lucide-user-x',
              color: 'error' as const,
              onSelect() {
                cancelTarget.value = row;
                cancelOpen.value = true;
              },
            },
          ]),
    ],
  ];
}

function statusLabel(status?: string | null) {
  const s = String(status || '');
  return statusItems.find((i) => i.value === s)?.label || s || '—';
}

function formatDate(v?: string | null) {
  if (!v) return '';
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return String(v);
  return d.toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function mapResultRow(r: any) {
  const enr = r.enrollment || r;
  const user = r.user || {};
  const ofoId = user.ofoId ?? (user.ofo != null && user.ofo !== '' && user.ofo !== '-1' ? Number(user.ofo) : null);
  return {
    ...r,
    id: enr.id ?? r.enrollmentId ?? r.id,
    fio: user.fio || r.fio || r.fullName || 'Сотрудник',
    login: user.login || r.login || '',
    ofoId: Number.isFinite(ofoId) ? ofoId : null,
    ofoName: displayOfoName(
      Number.isFinite(ofoId) ? ofoId : null,
      user.ofoName || r.ofoName || null,
    ),
    role: user.role || '',
    status: enr.status ?? r.status,
    progressPercent: enr.progress?.percent ?? r.progressPercent ?? 0,
    topicsCompleted: enr.progress?.topicsCompleted ?? null,
    topicsTotal: enr.progress?.topicsTotal ?? null,
    startedAt: enr.startedAt ?? null,
    lastActivityAt: enr.lastActivityAt ?? null,
    finalScore: enr.finalScore ?? r.finalScore,
    assignedAt: enr.assignedAt ?? r.assignedAt ?? null,
    deadlineAt: enr.deadlineAt ?? r.deadlineAt ?? null,
    completedAt: enr.completedAt ?? r.completedAt ?? null,
  };
}

async function exportResults() {
  exporting.value = true;
  try {
    await store.loadCourse(courseId.value);
    const data = (await store.loadResults({
      courseId: courseId.value,
      versionId: store.version.value?.id,
      q: searchQuery.value.trim() || undefined,
      ofoId: ofoFilter.value !== '_all' ? Number(ofoFilter.value) : undefined,
      status: statusFilter.value !== '_all' ? statusFilter.value : undefined,
      limit: 5000,
    })) as any;
    const list = (data?.items || []).map(mapResultRow);
    if (!list.length) {
      toast.add({
        title: 'Нечего выгружать',
        description: 'Нет участников по текущим фильтрам.',
        color: 'warning',
        icon: 'i-lucide-alert-triangle',
      });
      return;
    }

    const headers = [
      'ФИО',
      'Логин',
      'Должность',
      'ОФО',
      'Статус',
      'Прогресс %',
      'Темы пройдено',
      'Итоговый балл',
      'Назначен',
      'Начал',
      'Последняя активность',
      'Срок',
      'Завершён',
    ];
    const sheetData = [
      headers,
      ...list.map((r: any) => [
        r.fio,
        r.login || '',
        r.role || '',
        rootLabelOf(r.ofoId) || r.ofoName || '',
        statusLabel(r.status),
        r.progressPercent ?? 0,
        r.topicsTotal ? `${r.topicsCompleted ?? 0} из ${r.topicsTotal}` : '',
        r.finalScore ?? '',
        formatDate(r.assignedAt),
        formatDate(r.startedAt),
        formatDate(r.lastActivityAt),
        formatDate(r.deadlineAt),
        formatDate(r.completedAt),
      ]),
    ];

    const ws = XLSX.utils.aoa_to_sheet(sheetData);
    ws['!cols'] = [
      { wch: 32 },
      { wch: 16 },
      { wch: 24 },
      { wch: 28 },
      { wch: 14 },
      { wch: 12 },
      { wch: 14 },
      { wch: 14 },
      { wch: 18 },
      { wch: 18 },
      { wch: 20 },
      { wch: 18 },
      { wch: 18 },
    ];
    const wb = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(wb, ws, 'Результаты');

    const courseTitle = (store.current.value?.title || 'курс').replace(/[\\/:*?"<>|]+/g, '_').trim();
    const stamp = new Date().toISOString().slice(0, 10);
    XLSX.writeFile(wb, `Результаты_${courseTitle}_${stamp}.xlsx`);
    toast.add({
      title: 'Выгрузка готова',
      description: `Строк: ${list.length}`,
      color: 'success',
      icon: 'i-lucide-download',
    });
  } catch (e: any) {
    toast.add({ title: 'Не удалось выгрузить', description: e?.message, color: 'error', icon: 'i-lucide-x' });
  } finally {
    exporting.value = false;
  }
}

async function load() {
  loading.value = true;
  loadError.value = null;
  try {
    await store.loadCourse(courseId.value);
    const data = (await store.loadResults({
      courseId: courseId.value,
      versionId: store.version.value?.id,
      q: searchQuery.value.trim() || undefined,
      ofoId: ofoFilter.value !== '_all' ? Number(ofoFilter.value) : undefined,
      status: statusFilter.value !== '_all' ? statusFilter.value : undefined,
      limit: PAGE_LIMIT,
    })) as any;
    const agg = data?.aggregates || data?.summary || data?.stats || {};
    summary.value = {
      total: agg.total,
      // Сколько строк подходит под фильтр статуса (сводка его не учитывает).
      matched: agg.matched ?? agg.total,
      not_started: agg.notStarted ?? agg.not_started,
      completed: agg.completed,
      in_progress: agg.inProgress ?? agg.in_progress,
      overdue: agg.overdue,
      cancelled: agg.cancelled,
      avg_score: agg.avgScore ?? agg.avg_score,
    };
    rows.value = (data?.items || data?.rows || data?.participants || []).map(mapResultRow);
    if (selectedId.value && !rows.value.some((r) => r.id === selectedId.value)) {
      selectedId.value = null;
      participant.value = null;
    }
  } catch (e: any) {
    rows.value = [];
    loadError.value = e?.message || 'Не удалось загрузить отчёт';
  } finally {
    loading.value = false;
  }
}

/** Всего по фильтру больше, чем пришло строк, — говорим об этом, а не обрезаем молча. */
const truncated = computed(() => Number(summary.value.matched ?? 0) > rows.value.length);

/**
 * Карточки сводки: у каждой — число и доля от назначенных по текущему фильтру.
 * Отменённые назначения в знаменатель не входят: «Завершили 1 из 2», когда
 * третьему курс отменили, а не «из 3» (IMP-54).
 */
const summaryCards = computed(() => {
  const total = Math.max(
    0,
    Number(summary.value.total ?? rows.value.length) - Number(summary.value.cancelled ?? 0),
  );
  const card = (key: string, label: string, tone = '') => {
    const n = Number(summary.value[key] ?? 0);
    return { key, label, n, of: total, tone: n > 0 ? tone : '', active: statusFilter.value === key };
  };
  return [
    card('not_started', 'Не начали'),
    card('in_progress', 'В процессе'),
    card('overdue', 'Просрочили', 'text-error'),
    card('completed', 'Завершили', 'text-success'),
  ];
});

onMounted(async () => {
  await ensureOfo();
  await load();
  filtersReady = true;
  if (selectedId.value) await openDetail(selectedId.value);
});

watch([ofoFilter, statusFilter], () => {
  if (!filtersReady) return;
  void load();
});

watch(searchQuery, () => {
  if (!filtersReady) return;
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    void load();
  }, 300);
});

/** Клик по карточке сводки — показать только этот статус; повторный — снять фильтр. */
function toggleStatus(key: string) {
  statusFilter.value = statusFilter.value === key ? '_all' : key;
}

const cancelledCount = computed(() => Number(summary.value.cancelled ?? 0));

function clearFilters() {
  searchQuery.value = '';
  ofoFilter.value = '_all';
  statusFilter.value = '_all';
}

const detailPanel = ref<HTMLElement | null>(null);

async function openDetail(enrollmentId: number) {
  selectedId.value = enrollmentId;
  detailLoading.value = true;
  // На узком экране карточка стоит под списком — прокручиваем к ней.
  void nextTick(() => {
    if (window.matchMedia('(max-width: 1023px)').matches) {
      detailPanel.value?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  });
  try {
    participant.value = await store.loadParticipant({ enrollmentId, courseId: courseId.value });
  } catch (e: any) {
    toast.add({ title: 'Не удалось открыть карточку', description: e?.message, color: 'error', icon: 'i-lucide-x' });
    participant.value = null;
  } finally {
    detailLoading.value = false;
  }
}

async function openTestAnswers(t: any) {
  if (!selectedId.value || !t?.attemptId) return;
  answersTitle.value = t.title || 'Ответы теста';
  answersData.value = null;
  answersError.value = '';
  answersOpen.value = true;
  answersLoading.value = true;
  try {
    answersData.value = await store.loadAttemptAnswers({
      enrollmentId: selectedId.value,
      attemptId: t.attemptId,
    });
  } catch (e: any) {
    answersError.value = e?.message || 'Не удалось загрузить ответы';
  } finally {
    answersLoading.value = false;
  }
}

async function confirmCancel() {
  const row = cancelTarget.value;
  if (!row?.id) return;
  cancelling.value = true;
  try {
    await store.cancelEnrollment(row.id);
    toast.add({
      title: 'Назначение отменено',
      description: `Курс больше не показывается у сотрудника: ${row.fio || 'участник'}.`,
      color: 'success',
      icon: 'i-lucide-check',
    });
    cancelOpen.value = false;
    cancelTarget.value = null;
    await load();
    if (selectedId.value === row.id) {
      await openDetail(row.id);
    }
  } catch (e: any) {
    toast.add({
      title: 'Не удалось отменить',
      description: e?.message,
      color: 'error',
      icon: 'i-lucide-x',
    });
  } finally {
    cancelling.value = false;
  }
}

async function confirmReset() {
  const row = resetTarget.value;
  if (!row?.id) return;
  resetting.value = true;
  try {
    await store.resetEnrollment(row.id);
    toast.add({
      title: 'Результат обнулён',
      description: `${row.fio || 'Участник'} может пройти курс заново.`,
      color: 'success',
      icon: 'i-lucide-check',
    });
    resetOpen.value = false;
    resetTarget.value = null;
    await load();
    if (selectedId.value === row.id) {
      await openDetail(row.id);
    }
  } catch (e: any) {
    toast.add({
      title: 'Не удалось обнулить',
      description: e?.message,
      color: 'error',
      icon: 'i-lucide-x',
    });
  } finally {
    resetting.value = false;
  }
}
</script>

<template>
  <UMain class="relative w-full h-full min-h-0">
    <div class="flex flex-col gap-6 w-full h-full min-h-0 max-w-[1600px] mx-auto overflow-y-auto scrollbar-hide p-px pb-8 *:shrink-0">
    <UPageHeader
      headline="Обучение"
      title="Результаты"
      :description="store.current.value?.title || undefined"
    >
      <template #links>
        <UButton
          color="neutral"
          variant="outline"
          icon="i-lucide-download"
          :loading="exporting"
          :disabled="loading || Boolean(loadError)"
          @click="exportResults"
        >
          Выгрузить в Excel
        </UButton>
      </template>
    </UPageHeader>

    <div class="min-w-0 w-full flex flex-col gap-4 flex-1">
      <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
        <UInput
          v-model="searchQuery"
          icon="i-lucide-search"
          placeholder="Поиск по ФИО или логину…"
          size="lg"
          :ui="{ base: 'w-full' }"
        />
        <OfoSelect
          :model-value="ofoFilter === '_all' ? null : Number(ofoFilter)"
          placeholder="Все ОФО"
          reset-label="Все ОФО"
          size="lg"
          @update:model-value="ofoFilter = $event == null ? '_all' : String($event)"
        />
        <USelectMenu
          v-model="statusFilter"
          :items="statusItems"
          value-key="value"
          label-key="label"
          placeholder="Все статусы"
          size="lg"
          color="neutral"
          :search-input="false"
          class="w-full"
          :content="{ align: 'start', sideOffset: 8 }"
        />
        <USelectMenu
          v-model="sortKey"
          :items="sortItems"
          value-key="value"
          label-key="label"
          icon="i-lucide-arrow-down-up"
          size="lg"
          color="neutral"
          :search-input="false"
          class="w-full"
          aria-label="Сортировка"
          :content="{ align: 'start', sideOffset: 8 }"
        />
      </div>

      <div v-if="loading" class="flex flex-col gap-3" aria-busy="true" aria-label="Загрузка отчёта">
        <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
          <USkeleton v-for="n in 4" :key="n" class="h-20 w-full rounded-panel" />
        </div>
        <USkeleton v-for="n in 4" :key="`r${n}`" class="h-20 w-full rounded-panel" />
      </div>

      <UAlert
        v-else-if="loadError"
        color="warning"
        variant="subtle"
        icon="i-lucide-server"
        title="Не удалось загрузить отчёт"
        :description="loadError"
      >
        <template #actions>
          <UButton color="warning" icon="i-lucide-rotate-ccw" @click="load">Повторить</UButton>
        </template>
      </UAlert>

      <template v-else>
        <!-- Сводка: доля от назначенных (без отменённых); клик — фильтр по статусу -->
        <div class="grid grid-cols-2 gap-3" :class="hasFinalTest ? 'md:grid-cols-5' : 'md:grid-cols-4'">
          <button
            v-for="c in summaryCards"
            :key="c.key"
            type="button"
            class="rounded-panel bg-elevated p-3 min-w-0 text-left transition-colors hover:bg-accented/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :class="c.active ? 'ring-2 ring-inset ring-primary' : ''"
            :aria-pressed="c.active"
            :aria-label="`${c.label}: ${c.n} из ${c.of}. ${c.active ? 'Снять фильтр' : 'Показать только их'}`"
            @click="toggleStatus(c.key)"
          >
            <span class="block text-xs text-muted">{{ c.label }}</span>
            <span class="block text-xl font-semibold tabular-nums" :class="c.tone || 'text-highlighted'">
              {{ c.n }}<span class="text-sm font-normal text-muted"> из {{ c.of }}</span>
            </span>
          </button>
          <div v-if="hasFinalTest" class="rounded-panel bg-elevated p-3 min-w-0">
            <p class="text-xs text-muted">Средний итоговый балл</p>
            <p class="text-xl font-semibold tabular-nums text-highlighted">
              {{ summary.avg_score != null ? `${Math.round(Number(summary.avg_score))}%` : '—' }}
            </p>
          </div>
        </div>

        <div v-if="(truncated && rows.length) || (cancelledCount && statusFilter === '_all')" class="flex flex-col gap-1 -mt-1 text-sm text-muted">
          <p v-if="truncated && rows.length">
            Показаны последние {{ rows.length }} из {{ summary.matched }}. Уточните поиск или фильтры —
            либо выгрузите всех в Excel.
          </p>
          <p v-if="cancelledCount && statusFilter === '_all'">
            Отменённых назначений: {{ cancelledCount }} — они в конце списка и не входят в «из N».
          </p>
        </div>

        <UEmpty
          v-if="!rows.length"
          variant="naked"
          icon="i-lucide-bar-chart-3"
          :title="hasActiveFilters ? 'Никого не найдено' : 'Пока нет участников'"
          :description="hasActiveFilters
            ? 'Измените поиск, ОФО или статус.'
            : 'Назначьте курс сотрудникам — здесь появится их прогресс.'"
          class="py-10"
        >
          <template #actions>
            <UButton v-if="hasActiveFilters" color="neutral" variant="outline" icon="i-lucide-x" @click="clearFilters">
              Сбросить фильтры
            </UButton>
            <UButton
              v-else
              color="primary"
              icon="i-lucide-user-plus"
              :to="{ name: 'admin-course-assign', params: { courseId } }"
            >
              Назначить курс
            </UButton>
          </template>
        </UEmpty>

        <div v-else class="flex flex-col lg:flex-row lg:items-start gap-4 min-w-0">
          <ul class="flex-1 min-w-0 flex flex-col gap-2 list-none m-0 p-0.5">
            <li
              v-for="row in sortedRows"
              :key="row.id"
              class="rounded-panel ring-1 ring-inset ring-default flex items-center gap-1 pe-2 min-w-0 transition-colors hover:bg-elevated/50"
              :class="[
                selectedId === row.id ? 'ring-primary bg-elevated/50' : '',
                row.status === 'cancelled' ? 'opacity-60' : '',
              ]"
            >
              <button
                type="button"
                class="flex-1 min-w-0 flex items-center gap-3 p-3 text-left rounded-panel focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                :aria-pressed="selectedId === row.id"
                @click="openDetail(row.id)"
              >
                <span class="flex-1 min-w-0 flex flex-col gap-1">
                  <span class="flex flex-col sm:flex-row sm:items-baseline sm:gap-2 min-w-0">
                    <span class="font-medium text-highlighted break-words">{{ row.fio }}</span>
                    <span v-if="row.ofoName" class="text-xs text-muted break-words">{{ row.ofoName }}</span>
                    <span v-else class="text-xs text-dimmed">Подразделение не выбрано — скорее всего, ещё не входил на портал</span>
                  </span>
                  <span class="flex items-center gap-x-3 gap-y-1 flex-wrap text-xs text-muted">
                    <CourseStatusBadge :status="row.status" />
                    <span v-if="row.status !== 'cancelled'" class="inline-flex items-center gap-2 w-40 max-w-full">
                      <UProgress
                        :model-value="Number(row.progressPercent ?? 0)"
                        size="xs"
                        :color="row.status === 'completed' ? 'success' : 'primary'"
                        class="flex-1"
                        :aria-label="`Прогресс ${row.progressPercent ?? 0}%`"
                      />
                      <span class="tabular-nums shrink-0">
                        {{ row.topicsTotal ? `${row.topicsCompleted ?? 0}/${row.topicsTotal} тем` : `${row.progressPercent ?? 0}%` }}
                      </span>
                    </span>
                    <span
                      v-if="row.deadlineAt && row.status !== 'cancelled'"
                      :class="deadlineClass(describeDeadline(row.deadlineAt, row.status === 'completed')?.tone)"
                    >
                      {{ describeDeadline(row.deadlineAt, row.status === 'completed')?.label }}
                    </span>
                    <span v-if="row.status !== 'completed' && row.status !== 'cancelled'">
                      {{ row.lastActivityAt ? `Был в курсе ${formatLastActivity(row.lastActivityAt)}` : 'В курс не заходил' }}
                    </span>
                  </span>
                </span>
                <span
                  v-if="hasFinalTest"
                  class="text-sm tabular-nums shrink-0"
                  :class="row.finalScore == null ? 'text-dimmed' : 'text-highlighted font-medium'"
                  :title="row.finalScore == null ? 'Итогового балла нет' : 'Итоговый балл'"
                >
                  {{ row.finalScore == null ? '—' : scoreLabel(row.finalScore) }}
                </span>
              </button>
              <UTooltip text="Действия">
                <span class="inline-flex">
                  <UDropdownMenu
                    :items="rowMenuItems(row)"
                    :content="{ align: 'end' }"
                    @click.stop
                  >
                    <UButton
                      icon="i-lucide-ellipsis-vertical"
                      color="neutral"
                      variant="ghost"
                      square
                      size="sm"
                      aria-label="Действия"
                      @click.stop
                    />
                  </UDropdownMenu>
                </span>
              </UTooltip>
            </li>
          </ul>

          <aside
            v-if="selectedId"
            ref="detailPanel"
            class="w-full lg:w-[26rem] shrink-0 rounded-panel ring-1 ring-default p-4 flex flex-col gap-4 min-w-0 scroll-mt-4 lg:sticky lg:top-0 lg:max-h-[calc(100dvh-7rem)] lg:overflow-y-auto"
            aria-label="Карточка участника"
          >
            <div class="flex items-start justify-between gap-2">
              <div v-if="detail && !detailLoading" class="flex flex-col gap-0.5 min-w-0">
                <h2 class="text-lg font-medium text-highlighted break-words">{{ detail.fio }}</h2>
                <p v-if="detail.role" class="text-sm text-muted break-words">{{ detail.role }}</p>
                <p class="text-sm text-muted break-words">{{ detail.ofoName || 'Подразделение не выбрано' }}</p>
              </div>
              <h2 v-else class="text-lg font-medium">Участник</h2>
              <div class="flex items-center gap-1 shrink-0">
                <UTooltip v-if="detail" text="Действия">
                  <span class="inline-flex">
                    <UDropdownMenu
                      :items="rowMenuItems({ id: selectedId, fio: detail.fio, status: detail.status })"
                      :content="{ align: 'end' }"
                    >
                      <UButton
                        icon="i-lucide-ellipsis-vertical"
                        color="neutral"
                        variant="ghost"
                        square
                        size="sm"
                        aria-label="Действия"
                      />
                    </UDropdownMenu>
                  </span>
                </UTooltip>
                <UTooltip text="Закрыть карточку">
                  <UButton
                    color="neutral"
                    variant="ghost"
                    size="sm"
                    icon="i-lucide-x"
                    aria-label="Закрыть карточку"
                    @click="selectedId = null; participant = null" />
                </UTooltip>
              </div>
            </div>

            <div v-if="detailLoading" class="flex flex-col gap-3" aria-busy="true" aria-label="Загрузка карточки">
              <USkeleton class="h-5 w-2/3 rounded-lg" />
              <USkeleton class="h-16 w-full rounded-lg" />
              <USkeleton class="h-32 w-full rounded-lg" />
            </div>
            <template v-else-if="detail">
              <div v-if="detail.email || detail.phone" class="flex flex-wrap gap-2">
                <UButton
                  v-if="detail.email"
                  :href="`mailto:${detail.email}`"
                  color="neutral"
                  variant="soft"
                  size="sm"
                  icon="i-lucide-mail"
                >
                  {{ detail.email }}
                </UButton>
                <UButton
                  v-if="detail.phone"
                  :href="`tel:${detail.phone}`"
                  color="neutral"
                  variant="soft"
                  size="sm"
                  icon="i-lucide-phone"
                >
                  {{ detail.phone }}
                </UButton>
              </div>

              <div class="flex flex-col gap-2">
                <div class="flex items-center justify-between gap-2 flex-wrap">
                  <CourseStatusBadge :status="detail.status" />
                  <span class="text-sm text-muted tabular-nums">
                    <template v-if="detail.topicsTotal">Тем {{ detail.topicsCompleted }} из {{ detail.topicsTotal }} · </template>{{ detail.progressPercent }}%
                  </span>
                </div>
                <UProgress
                  :model-value="Number(detail.progressPercent || 0)"
                  size="sm"
                  :color="detail.status === 'completed' ? 'success' : 'primary'"
                  :aria-label="`Прогресс ${detail.progressPercent}%`"
                />
                <p v-if="hasFinalTest && detail.finalScore != null" class="text-sm">
                  Итоговый балл: <span class="font-medium tabular-nums">{{ scoreLabel(detail.finalScore) }}</span>
                </p>
              </div>

              <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm m-0">
                <template v-for="d in detail.dates" :key="d.label">
                  <dt class="text-muted">{{ d.label }}</dt>
                  <dd class="m-0 text-default">{{ d.value }}</dd>
                </template>
                <template v-if="detail.deadline">
                  <dt class="text-muted">Срок</dt>
                  <dd class="m-0" :class="deadlineClass(detail.deadline.tone)" :title="detail.deadline.full">
                    {{ detail.deadline.label }}
                  </dd>
                </template>
                <template v-if="detail.totalSeconds">
                  <dt class="text-muted">Время в курсе</dt>
                  <dd class="m-0 text-default tabular-nums">{{ fmtDuration(detail.totalSeconds) }}</dd>
                </template>
              </dl>

              <USeparator />

              <div class="flex flex-col gap-2 min-w-0">
                <h3 class="text-sm font-medium text-highlighted">Тесты</h3>
                <p v-if="detail.tests.some((t: any) => t.attemptId)" class="text-xs text-dimmed">
                  Нажмите на тест, чтобы открыть ответы по вопросам
                </p>
                <UEmpty
                  v-if="!detail.tests.length"
                  variant="naked"
                  icon="i-lucide-clipboard-list"
                  title="Тестов в курсе нет"
                  class="py-4"
                />
                <ul v-else class="flex flex-col gap-2 list-none m-0 p-0">
                  <li v-for="t in detail.tests" :key="t.courseTestLinkId">
                    <!-- С попыткой — кнопка (доступна с клавиатуры), без попытки — просто карточка -->
                    <component
                      :is="t.attemptId ? 'button' : 'div'"
                      :type="t.attemptId ? 'button' : undefined"
                      class="w-full text-left rounded-lg ring-1 ring-inset ring-default p-3 flex flex-col gap-1.5 min-w-0"
                      :class="t.attemptId ? 'hover:bg-elevated/50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary' : ''"
                      :aria-label="t.attemptId ? `Ответы: ${t.title}` : undefined"
                      @click="t.attemptId ? openTestAnswers(t) : undefined"
                    >
                    <span class="flex items-start justify-between gap-2">
                      <span class="block min-w-0">
                        <span class="block text-sm font-medium text-highlighted break-words">{{ t.title }}</span>
                        <span class="block text-xs text-muted break-words">{{ testKindLabel(t) }}</span>
                      </span>
                      <span class="flex items-center gap-1 shrink-0">
                        <UBadge :color="testResultColor(t)" variant="subtle">
                          {{ testResultLabel(t) }}
                        </UBadge>
                        <UIcon
                          v-if="t.attemptId"
                          name="i-lucide-eye"
                          class="size-4 text-dimmed"
                          aria-hidden="true"
                        />
                      </span>
                    </span>
                    <span class="flex items-center gap-3 text-xs text-muted flex-wrap">
                      <span v-if="t.score != null" class="tabular-nums">Результат: {{ scoreLabel(t.score) }}</span>
                      <span v-if="t.attemptsCount" class="tabular-nums">Попыток: {{ t.attemptsCount }}</span>
                      <span v-else>Попыток не было</span>
                      <span v-if="t.finishedAt">{{ formatLastActivity(t.finishedAt) }}</span>
                    </span>
                    </component>
                  </li>
                </ul>
              </div>

              <template v-if="detail.topics?.length">
                <USeparator />
                <div class="flex flex-col gap-2 min-w-0">
                  <h3 class="text-sm font-medium text-highlighted">Темы</h3>
                  <ul class="flex flex-col gap-2.5 list-none m-0 p-0">
                    <li
                      v-for="tp in detail.topics"
                      :key="tp.topicId"
                      class="flex items-start justify-between gap-2 text-sm min-w-0"
                    >
                      <span class="min-w-0 flex flex-col">
                        <span class="break-words">{{ tp.title }}</span>
                        <span class="text-xs text-muted tabular-nums">
                          <template v-if="tp.materialsTotal">Материалы {{ tp.materialsDone }} из {{ tp.materialsTotal }}</template>
                          <template v-if="tp.activeSeconds"> · {{ fmtDuration(Number(tp.activeSeconds)) }}</template>
                        </span>
                      </span>
                      <CourseStatusBadge :status="tp.status" class="shrink-0" />
                    </li>
                  </ul>
                </div>
              </template>
            </template>
          </aside>
        </div>
      </template>
    </div>

    </div>

    <UModal
      v-model:open="answersOpen"
      :title="`Ответы — ${answersTitle}`"
      description=""
      :ui="{ content: 'max-w-2xl' }"
    >
      <template #body>
        <div class="max-h-[70vh] overflow-y-auto px-1 py-1 flex flex-col gap-3">
          <div v-if="answersLoading" class="flex flex-col gap-3" aria-busy="true" aria-label="Загрузка ответов">
            <USkeleton class="h-6 w-40 rounded-lg" />
            <USkeleton v-for="n in 3" :key="n" class="h-20 w-full rounded-xl" />
          </div>
          <div v-else-if="answersError" class="py-8 text-center text-error text-sm">{{ answersError }}</div>
          <template v-else-if="answersData">
            <div class="flex items-center gap-2 flex-wrap">
              <UBadge
                v-if="answersData.attempt?.score != null"
                :color="answersData.attempt.passed === false ? 'error' : 'success'"
                variant="subtle"
                class="tabular-nums"
              >
                {{ Math.round(Number(answersData.attempt.score)) }}%
              </UBadge>
              <UBadge
                v-if="answersData.attempt?.passed != null"
                :color="answersData.attempt.passed ? 'success' : 'error'"
                variant="subtle"
              >
                {{ answersData.attempt.passed ? 'Тест пройден' : 'Не пройден' }}
              </UBadge>
              <span
                v-if="answersData.attempt?.durationSec != null"
                class="text-xs text-dimmed"
              >
                время: {{ fmtDuration(Number(answersData.attempt.durationSec)) }}
              </span>
            </div>
            <div
              v-for="(a, i) in (answersData.answers || [])"
              :key="i"
              class="rounded-xl ring-1 p-3 flex flex-col gap-1"
              :class="a.isCorrect === false
                ? 'ring-error/40 bg-error/5'
                : (a.isCorrect === true ? 'ring-success/40 bg-success/5' : 'ring-default')"
            >
              <div class="flex items-center gap-2">
                <UIcon
                  v-if="a.isCorrect === true"
                  name="i-lucide-check-circle-2"
                  class="size-4 text-success shrink-0"
                  aria-hidden="true"
                />
                <UIcon
                  v-else-if="a.isCorrect === false"
                  name="i-lucide-x-circle"
                  class="size-4 text-error shrink-0"
                  aria-hidden="true"
                />
                <span class="text-sm text-highlighted">{{ i + 1 }}. {{ a.title || 'Без названия' }}</span>
              </div>
              <p class="text-sm text-muted pl-6 whitespace-pre-line">Ответ: {{ a.userAnswer }}</p>
              <p
                v-if="a.isCorrect === false && a.correctAnswer"
                class="text-sm text-success pl-6 whitespace-pre-line"
              >
                Правильно: {{ a.correctAnswer }}
              </p>
            </div>
            <p v-if="!(answersData.answers || []).length" class="text-sm text-muted py-4 text-center">
              Ответов в попытке нет.
            </p>
          </template>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="resetOpen"
      title="Обнулить результат?"
      description="Прогресс, материалы, попытки тестов и завершение будут удалены. Участник сможет пройти курс заново."
    >
      <template #body>
        <p class="text-sm text-muted">
          Участник:
          <span class="text-highlighted font-medium">{{ resetTarget?.fio || '—' }}</span>
        </p>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton color="neutral" variant="ghost" @click="resetOpen = false">Отмена</UButton>
          <UButton color="error" icon="i-lucide-rotate-ccw" :loading="resetting" @click="confirmReset">
            Обнулить
          </UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="cancelOpen"
      title="Отменить назначение?"
      description="Курс пропадёт из «Моего обучения» сотрудника. Прогресс сохранится в результатах со статусом «Отменён»; назначить курс заново можно в любой момент."
    >
      <template #body>
        <p class="text-sm text-muted">
          Участник:
          <span class="text-highlighted font-medium">{{ cancelTarget?.fio || '—' }}</span>
        </p>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton color="neutral" variant="ghost" @click="cancelOpen = false">Отмена</UButton>
          <UButton color="error" icon="i-lucide-user-x" :loading="cancelling" @click="confirmCancel">
            Отменить назначение
          </UButton>
        </div>
      </template>
    </UModal>
  </UMain>
</template>
