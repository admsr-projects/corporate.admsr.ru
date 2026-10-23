<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useCoursesStore } from '../../../composables/useCoursesStore';
import { useAppToast } from '../../../composables/useAppToast';
import {
  useBreadcrumbCurrentLabel,
  useBreadcrumbLabelsByRoute,
} from '../../../composables/usePortalNavigation';
import { createEmptyForm, type TestForm } from '../../Tests/testForm';
import TestRunner from '../../Tests/TestRunner.vue';
import { followCourseNextAction } from '../followNextAction';

const route = useRoute();
const router = useRouter();
const store = useCoursesStore();
const { toast } = useAppToast();
const breadcrumbLabel = useBreadcrumbCurrentLabel();
const breadcrumbByRoute = useBreadcrumbLabelsByRoute();

const enrollmentId = computed(() => Number(route.params.enrollmentId));
const courseTestLinkId = computed(() => Number(route.params.courseTestLinkId));

const loading = ref(true);
const loadError = ref<string | null>(null);
const form = reactive<TestForm>(createEmptyForm());
const attemptId = ref<number | null>(null);
const ready = ref(false);
const savedAnswers = ref<Record<string, unknown>>({});
const runnerKey = ref(0);
const retaking = ref(false);
/**
 * Попытка уже завершена на сервере. После «Закрыть» на экране результата раннер
 * возвращается к «Пройти тест», а ответы раньше уходили в эту же завершённую попытку:
 * сервер отвечал 409 «Попытка уже завершена», а до ADR-057 сотрудник видел «0 из 0»
 * и пересдать не мог. Теперь новое прохождение заводит новую попытку (ADR-058).
 */
const attemptDone = ref(false);
let renewing: Promise<void> | null = null;
const courseTitle = ref('Обучение');

watch(
  [courseTitle, () => form.title],
  ([course, testTitle]) => {
    breadcrumbByRoute.value = {
      ...breadcrumbByRoute.value,
      'course-enrollment': course,
    };
    breadcrumbLabel.value = String(testTitle || 'Тест');
  },
  { immediate: true },
);

onUnmounted(() => {
  breadcrumbLabel.value = null;
  const next = { ...breadcrumbByRoute.value };
  delete next['course-enrollment'];
  breadcrumbByRoute.value = next;
});

async function beginAttempt(opts?: { clearAnswers?: boolean }) {
  const started = (await store.attemptStart({
    enrollmentId: enrollmentId.value,
    courseTestLinkId: courseTestLinkId.value,
  })) as any;

  attemptId.value = Number(started?.attemptId ?? started?.attempt?.id);
  attemptDone.value = false;
  const src = started?.form;
  if (src) {
    Object.assign(form, createEmptyForm(), src, {
      kind: 'test',
      questions: Array.isArray(src.questions) ? src.questions : [],
    });
  } else {
    const got = (await store.getCourseTest({ courseTestLinkId: courseTestLinkId.value })) as any;
    if (got?.form) {
      Object.assign(form, createEmptyForm(), got.form, {
        kind: 'test',
        questions: Array.isArray(got.form.questions) ? got.form.questions : [],
      });
    }
  }

  if (opts?.clearAnswers) {
    savedAnswers.value = {};
  } else if (attemptId.value) {
    try {
      const existing = (await store.attemptGet({ attemptId: attemptId.value })) as any;
      if (existing?.answers && typeof existing.answers === 'object') {
        savedAnswers.value = existing.answers;
      } else {
        savedAnswers.value = {};
      }
    } catch {
      savedAnswers.value = {};
    }
  }
  ready.value = true;
}

onMounted(() => {
  void loadAttempt();
});

watch(courseTestLinkId, (id, prev) => {
  if (!id || id === prev) return;
  void loadAttempt();
});

async function loadAttempt() {
  loading.value = true;
  loadError.value = null;
  ready.value = false;
  try {
    const enrollment = await store.getEnrollment(enrollmentId.value).catch(() => null) as any;
    courseTitle.value =
      enrollment?.enrollment?.course?.title
      || enrollment?.course?.title
      || enrollment?.version?.title
      || 'Обучение';
    await beginAttempt();
  } catch (e: any) {
    // Ошибка остаётся на экране в UAlert — тост поверх дублировал бы её.
    loadError.value = e?.message || 'Не удалось начать тест';
  } finally {
    loading.value = false;
  }
}

/** Новая попытка вместо завершённой — без перезагрузки формы, ответы остаются в раннере. */
async function renewAttempt() {
  const started = (await store.attemptStart({
    enrollmentId: enrollmentId.value,
    courseTestLinkId: courseTestLinkId.value,
  })) as any;
  attemptId.value = Number(started?.attemptId ?? started?.attempt?.id);
  attemptDone.value = false;
}

function onRunnerStart() {
  if (!attemptDone.value || renewing) return;
  renewing = renewAttempt()
    .catch((e: any) => {
      toast.add({
        title: 'Не удалось начать новую попытку',
        description: e?.message || 'Возможно, исчерпан лимит попыток.',
        color: 'error',
        icon: 'i-lucide-alert-circle',
      });
    })
    .finally(() => {
      renewing = null;
    });
}

async function onSubmit(answers: Record<string, unknown>, durationSec: number) {
  if (renewing) await renewing;
  // Не успели завести новую попытку при старте (или не вышло) — заводим сейчас; ошибка
  // (например, лимит попыток) покажется в раннере, ответы останутся.
  if (attemptDone.value) await renewAttempt();
  if (!attemptId.value) throw new Error('Нет попытки');
  await store.attemptSave({
    attemptId: attemptId.value,
    answers,
    enrollmentId: enrollmentId.value,
    courseTestLinkId: courseTestLinkId.value,
  });
  const result = await store.attemptFinish({
    attemptId: attemptId.value,
    answers,
    durationSec,
    enrollmentId: enrollmentId.value,
    courseTestLinkId: courseTestLinkId.value,
  });
  attemptDone.value = true;
  return result;
}

function onFinish(payload?: {
  result?: { score?: number | null; passed?: boolean | null; correctCount?: number; scorable?: number } | null;
}) {
  const r = payload?.result;
  if (r?.passed === false) {
    const score = r.score != null ? `${Math.round(Number(r.score))}%` : null;
    toast.add({
      title: 'Тест не сдан',
      description: score
        ? `Ваш результат: ${score}. Можно пройти тест снова.`
        : 'Попробуйте ещё раз.',
      color: 'error',
      icon: 'i-lucide-x',
    });
    return;
  }
  if (r?.passed === true) {
    const score = r.score != null ? `${Math.round(Number(r.score))}%` : null;
    toast.add({
      title: 'Тест сдан',
      description: score ? `Результат: ${score}` : undefined,
      color: 'success',
      icon: 'i-lucide-check',
    });
  } else {
    toast.add({ title: 'Тест завершён', color: 'success', icon: 'i-lucide-check' });
  }
  void goNextAfterPass();
}

async function goNextAfterPass() {
  try {
    const raw = (await store.nextAction(enrollmentId.value)) as { nextAction?: { type?: string; topicId?: number; courseTestLinkId?: number } };
    await followCourseNextAction(router, enrollmentId.value, raw?.nextAction ?? raw);
  } catch {
    await router.push({ name: 'course-enrollment', params: { enrollmentId: enrollmentId.value } });
  }
}

async function onRetake() {
  retaking.value = true;
  try {
    await beginAttempt({ clearAnswers: true });
    runnerKey.value += 1;
  } catch (e: any) {
    toast.add({
      title: 'Не удалось начать заново',
      description: e?.message || 'Возможно, исчерпан лимит попыток.',
      color: 'error',
      icon: 'i-lucide-alert-circle',
    });
  } finally {
    retaking.value = false;
  }
}
</script>

<template>
  <UMain class="flex flex-1 flex-col w-full min-w-0 h-full min-h-0 gap-3 overflow-x-hidden overflow-y-auto">
    <h1 class="sr-only">{{ form.title || 'Прохождение теста' }}</h1>

    <div v-if="loading || retaking" class="flex flex-col gap-3 flex-1">
      <USkeleton class="h-full min-h-64 w-full rounded-xl" />
    </div>

    <UAlert
      v-else-if="!ready"
      color="warning"
      variant="subtle"
      icon="i-lucide-server"
      title="Тест недоступен"
      :description="loadError || 'Вернитесь к курсу и попробуйте снова.'"
    >
      <template #actions>
        <UButton color="warning" icon="i-lucide-rotate-ccw" @click="loadAttempt">Повторить</UButton>
        <UButton color="neutral" variant="ghost" :to="{ name: 'course-enrollment', params: { enrollmentId } }">
          К курсу
        </UButton>
      </template>
    </UAlert>

    <TestRunner
      v-else
      :key="runnerKey"
      :form="form"
      :initial-answers="Object.keys(savedAnswers).length ? savedAnswers : null"
      :submit="onSubmit"
      class="flex-1 min-h-0"
      @finish="onFinish"
      @retake="onRetake"
      @start="onRunnerStart"
    />
  </UMain>
</template>
