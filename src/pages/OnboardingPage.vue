<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useOfoTree } from '../composables/useOfoTree';
import UserWorkFields from '../components/UserWorkFields.vue';
import { userShortName } from '../utils/userName';
import { useProfileDisplay } from '../composables/useProfileDisplay';
import { useAppToast } from '../composables/useAppToast';
import { clearAuthStorage } from '../composables/useAuthSession';
import { patchAppTheme, useAppConfig } from '../composables/useAppConfig';
import {
  COLOR_MODE_KEY,
  readMainColorModePreference,
  resolveMainColorMode,
  type ColorModeResolved,
} from '../composables/useColorMode';
import {
  FONT_OPTIONS,
  PRIMARY_COLORS,
  PRIMARY_COLOR_LABELS,
  RADIUS_OPTIONS,
} from '../composables/useUiTheme';
import {
  avatarUrlFromFilename,
  PROFILE_AVATAR_FILENAMES,
} from '../constants/profileAvatars';
import {
  defaultAvatarUrl,
  expireSession,
  loadProfileSnapshot,
  isOfoUnset,
  isProfileIncomplete,
  markOnboardingComplete,
  saveOnboardingProfile,
  type ProfileSnapshot,
} from '../composables/useOnboarding';

const router = useRouter();
const { success, error } = useAppToast();
const { setAvatarSrc, setDisplayName, setSubtitle } = useProfileDisplay();

const { ensureLoaded: ensureOfoLoaded, pathLabel } = useOfoTree();
ensureOfoLoaded();


const STEPS = [
  { id: 'welcome', label: 'Приветствие', icon: 'i-lucide-sparkles', date: 'Шаг 1' },
  { id: 'work', label: 'Место работы', icon: 'i-lucide-building-2', date: 'Шаг 2' },
  { id: 'avatar', label: 'Аватар', icon: 'i-lucide-smile', date: 'Шаг 3' },
  { id: 'look', label: 'Внешний вид', icon: 'i-lucide-palette', date: 'Шаг 4' },
  { id: 'done', label: 'Готово', icon: 'i-lucide-check-circle', date: 'Шаг 5' },
] as const;

type StepId = (typeof STEPS)[number]['id'];

const currentStep = ref<StepId>('welcome');
const stepIndex = computed(() => STEPS.findIndex((s) => s.id === currentStep.value));

type StepVisualState = 'completed' | 'active' | 'upcoming';

function stepVisualState(index: number): StepVisualState {
  if (index < stepIndex.value) return 'completed';
  if (index === stepIndex.value) return 'active';
  return 'upcoming';
}

function leftSegmentClass(index: number): string {
  if (index === 0) return 'opacity-0 pointer-events-none';
  return index <= stepIndex.value ? 'bg-primary' : 'bg-elevated';
}

function rightSegmentClass(index: number): string {
  if (index === STEPS.length - 1) return 'opacity-0 pointer-events-none';
  return index < stepIndex.value ? 'bg-primary' : 'bg-elevated';
}

function indicatorClass(index: number): string {
  const state = stepVisualState(index);
  if (state === 'upcoming') {
    return 'bg-elevated text-muted ring-1 ring-default';
  }
  return 'bg-primary text-inverted ring-2 ring-primary/30';
}

function goToTimelineStep(stepId: StepId, index: number) {
  if (index <= stepIndex.value) {
    currentStep.value = stepId;
  }
}

const profile = ref<ProfileSnapshot | null>(null);
const profileLoading = ref(true);

const form = reactive({
  ofoId: null as number | null,
  role: '',
  avatarUrl: defaultAvatarUrl(),
});

const saving = ref(false);
const loggingOut = ref(false);

async function logout() {
  loggingOut.value = true;
  try {
    const user = JSON.parse(localStorage.getItem('auth-user') || 'null') as { id?: number } | null;
    if (user?.id) {
      await fetch('/api/logout.php', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: user.id }),
      });
    }
  } catch {
    /* сеть недоступна — всё равно выходим локально */
  } finally {
    // Как в меню профиля: без этого токен сессии оставался в localStorage.
    clearAuthStorage();
    loggingOut.value = false;
    await router.replace({ name: 'login' });
  }
}

const portalFeatures = [
  {
    icon: 'i-lucide-newspaper',
    title: 'Новости и мероприятия',
    description: 'Лента новостей с реакциями, афиша мероприятий и фотогалерея.',
  },
  {
    icon: 'i-lucide-graduation-cap',
    title: 'Обучение',
    description: 'Назначенные курсы: материалы, видео и тесты. Прогресс и сроки — в разделе «Обучение».',
  },
  {
    icon: 'i-lucide-calendar-off',
    title: 'Журнал отсутствия',
    description: 'Отметьте выезд, совещание или другое отсутствие — коллеги увидят, где вы.',
  },
  {
    icon: 'i-lucide-clipboard-list',
    title: 'Формы и опросы',
    description: 'Анкеты, опросы и тесты, которые направили вам или вашему подразделению.',
  },
  {
    icon: 'i-lucide-cake',
    title: 'Дни рождения',
    description: 'Именинники на главной — не пропускайте важные даты.',
  },
  {
    icon: 'i-lucide-user-circle',
    title: 'Профиль',
    description: 'Ваши данные, аватар и стена с публикациями.',
  },
];


/** ОФО уже задано (остался пустым только аватар или должность) — менять его на портале нельзя. */
const ofoAlreadySet = computed(() => !!profile.value && !isOfoUnset(profile.value.ofo));
const selectedOfoLabel = computed(() => pathLabel(form.ofoId));

// ── Внешний вид ──────────────────────────────────────────────────────────────
// Настройки живут в этом браузере (localStorage) и применяются сразу — их видно
// на самой странице. Те же переключатели остаются в меню профиля.
const appConfig = useAppConfig();

/**
 * Светлая / тёмная схема. Состоянием владеет App.vue (useColorMode): сохраняем
 * выбор в тот же ключ и сообщаем событием, которое App.vue уже слушает.
 */
const colorMode = ref<ColorModeResolved>(resolveMainColorMode(readMainColorModePreference()));
function setColorMode(mode: ColorModeResolved) {
  colorMode.value = mode;
  try {
    localStorage.setItem(COLOR_MODE_KEY, mode);
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new CustomEvent('ui-color-mode-change', { detail: { mode, preference: mode } }));
}

const primaryLabel = computed(() => PRIMARY_COLOR_LABELS[appConfig.ui.colors.primary]);
const fontLabel = computed(() => FONT_OPTIONS.find((f) => f.id === appConfig.ui.font)?.label ?? '');
const radiusLabel = computed(() => RADIUS_OPTIONS.find((r) => r.id === appConfig.ui.radius)?.label ?? '');
const lookSummary = computed(() => `${primaryLabel.value} · ${fontLabel.value} · скругление: ${radiusLabel.value}`);

const canProceedFromWork = computed(
  () => form.ofoId != null && Boolean(form.role.trim()),
);

const canProceedFromAvatar = computed(() => Boolean(String(form.avatarUrl ?? '').trim()));

const greetingName = computed(() => {
  const p = profile.value;
  if (!p) return 'коллега';
  const namePatronymic = [p.firstname, p.lastname].filter(Boolean).join(' ');
  return namePatronymic || 'коллега';
});

function goNext() {
  const idx = stepIndex.value;
  if (idx < 0 || idx >= STEPS.length - 1) return;
  currentStep.value = STEPS[idx + 1].id;
}

function goBack() {
  const idx = stepIndex.value;
  if (idx <= 0) return;
  currentStep.value = STEPS[idx - 1].id;
}

function onWorkNext() {
  if (!canProceedFromWork.value) {
    error('Заполните данные', 'Выберите подразделение и должность, чтобы продолжить.');
    return;
  }
  goNext();
}

function onAvatarNext() {
  if (!canProceedFromAvatar.value) {
    error('Выберите аватар', 'Нажмите на изображение, которое будет отображаться в профиле.');
    return;
  }
  goNext();
}

function selectAvatar(filename: string) {
  form.avatarUrl = avatarUrlFromFilename(filename);
}

async function finishOnboarding() {
  const p = profile.value;
  if (!p?.id) return;

  saving.value = true;
  try {
    const ok = await saveOnboardingProfile({
      id: p.id,
      ofo: String(form.ofoId ?? ''),
      role: form.role,
      avatar_url: form.avatarUrl,
    });

    if (!ok) {
      error('Не удалось сохранить', 'Проверьте подключение и попробуйте снова.');
      return;
    }

    setAvatarSrc(form.avatarUrl);
    setSubtitle(form.role);
    setDisplayName(userShortName(p));

    const raw = localStorage.getItem('auth-user');
    if (raw) {
      try {
        const user = JSON.parse(raw) as Record<string, unknown>;
        user.ofo = form.ofoId;
        user.role = form.role;
        localStorage.setItem('auth-user', JSON.stringify(user));
      } catch {
        /* ignore */
      }
    }

    markOnboardingComplete(p.id);
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new Event('ui:user-profile-updated'));
    }

    success('Добро пожаловать!', 'Профиль настроен — можно пользоваться порталом.');
    await router.replace({ name: 'home' });
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  let userId = 0;
  try {
    const raw = localStorage.getItem('auth-user');
    const user = raw ? (JSON.parse(raw) as { id?: number }) : null;
    userId = Number(user?.id ?? 0);
  } catch {
    userId = 0;
  }

  if (!userId) {
    await router.replace({ name: 'login' });
    return;
  }

  profileLoading.value = true;
  try {
    const res = await loadProfileSnapshot(userId);
    if (res.status !== 'ok') {
      // Сессия истекла — на вход с «Сессия истекла», а не пустое приветствие.
      if (res.status === 'unauthorized') expireSession();
      await router.replace({ name: 'login' });
      return;
    }
    const snap = res.profile;
    if (!isProfileIncomplete(snap)) {
      markOnboardingComplete(userId);
      await router.replace({ name: 'home' });
      return;
    }

    profile.value = snap;
    const ofoNum = Number(snap.ofo);
    if (!isOfoUnset(snap.ofo) && Number.isFinite(ofoNum) && ofoNum > 0) form.ofoId = ofoNum;
    if (snap.role) form.role = snap.role;
    if (snap.avatar_url) form.avatarUrl = snap.avatar_url;
  } finally {
    profileLoading.value = false;
  }
});
</script>

<template>
  <!--
    Корень приложения — h-dvh overflow-hidden, документ не скроллится. Шаг «Внешний вид»
    выше экрана, поэтому страница скроллит себя сама (my-auto у содержимого —
    центр, когда помещается, и без обрезки сверху, когда нет).
  -->
  <div class="relative flex h-full min-h-0 flex-col items-center overflow-y-auto bg-(--ui-bg) px-4 py-8 sm:py-11">
    <div
      class="pointer-events-none fixed inset-0 opacity-40 dark:opacity-25"
      aria-hidden="true"
      style="background-image: radial-gradient(circle at 15% 20%, color-mix(in oklab, var(--ui-color-primary-500) 35%, transparent) 0%, transparent 45%), radial-gradient(circle at 85% 75%, color-mix(in oklab, var(--ui-color-violet-500) 25%, transparent) 0%, transparent 40%);"
    />

    <div class="relative z-10 my-auto flex w-full max-w-4xl flex-col gap-6">
      <div class="flex flex-col items-center gap-2 text-center">
        <p
          class="text-[clamp(18px,3vw,28px)] leading-tight text-(--ui-text-highlighted)"
          style="font-family: 'Unbounded', sans-serif;"
        >
          <span class="font-bold">ADMSR</span>
          <span class="font-light"> | КОРПОРАТИВНЫЙ ПОРТАЛ</span>
        </p>
      </div>

      <UCard class="w-full shadow-lg ring-1 ring-default">
        <div class="flex flex-col gap-6 p-1 sm:p-2">
          <nav
            aria-label="Шаги настройки профиля"
            class="w-full px-2 sm:px-6 pt-2 pb-1"
          >
            <ol class="m-0 grid w-full list-none grid-cols-5 gap-0 p-0">
              <li
                v-for="(step, index) in STEPS"
                :key="step.id"
                class="flex min-w-0 flex-col items-center"
              >
                <div class="flex w-full items-center">
                  <div
                    class="h-0.5 min-w-2 flex-1 rounded-full transition-colors duration-300"
                    :class="leftSegmentClass(index)"
                    aria-hidden="true"
                  />

                  <button
                    type="button"
                    class="shrink-0 rounded-full transition-transform focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-default"
                    :aria-current="stepVisualState(index) === 'active' ? 'step' : undefined"
                    :aria-label="`${step.date}: ${step.label}`"
                    :disabled="index > stepIndex"
                    @click="goToTimelineStep(step.id, index)"
                  >
                    <UAvatar
                      :icon="step.icon"
                      size="xl"
                      :class="[
                        'size-14 sm:size-16 [&_[data-slot=icon]]:size-7 sm:[&_[data-slot=icon]]:size-8',
                        indicatorClass(index),
                      ]"
                      :ui="{ root: '!rounded-full', icon: 'text-inherit' }"
                    />
                  </button>

                  <div
                    class="h-0.5 min-w-2 flex-1 rounded-full transition-colors duration-300"
                    :class="rightSegmentClass(index)"
                    aria-hidden="true"
                  />
                </div>

                <div class="mt-3 w-full px-1 text-center">
                  <p class="text-xs text-muted tabular-nums sm:text-sm">
                    {{ step.date }}
                  </p>
                  <p
                    class="mt-0.5 text-sm font-medium leading-snug text-balance sm:text-base"
                    :class="stepVisualState(index) === 'upcoming' ? 'text-muted' : 'text-highlighted'"
                  >
                    {{ step.label }}
                  </p>
                </div>
              </li>
            </ol>
          </nav>

          <USkeleton v-if="profileLoading" class="h-64 w-full" />

          <template v-else>
            <!-- Шаг 1: приветствие -->
            <section
              v-show="currentStep === 'welcome'"
              class="flex flex-col gap-5"
              aria-labelledby="onboarding-welcome-title"
            >
              <div class="text-center space-y-2">
                <h1
                  id="onboarding-welcome-title"
                  class="text-2xl sm:text-3xl font-semibold text-highlighted font-unbounded"
                >
                  Здравствуйте, {{ greetingName }}!
                </h1>
                <p class="text-base text-muted max-w-xl mx-auto">
                  Мы обновили корпоративный портал ADMSR. За пару шагов настроим профиль и внешний вид —
                  цвета, шрифт, скругление и масштаб страницы. Ниже — чем можно пользоваться каждый день.
                </p>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <UCard
                  v-for="feature in portalFeatures"
                  :key="feature.title"
                  variant="subtle"
                  class="ring-1 ring-default"
                >
                  <div class="flex gap-3 p-1">
                    <div
                      class="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"
                    >
                      <UIcon :name="feature.icon" class="size-5" />
                    </div>
                    <div class="min-w-0 text-left">
                      <p class="font-medium text-highlighted text-sm">
                        {{ feature.title }}
                      </p>
                      <p class="text-xs text-muted mt-0.5 leading-relaxed">
                        {{ feature.description }}
                      </p>
                    </div>
                  </div>
                </UCard>
              </div>

              <div class="flex flex-wrap items-center justify-between gap-3 pt-2">
                <UButton
                  type="button"
                  size="xl"
                  color="neutral"
                  variant="outline"
                  icon="i-lucide-log-out"
                  label="Выйти"
                  :loading="loggingOut"
                  @click="logout"
                />
                <UButton
                  size="xl"
                  trailing-icon="i-lucide-arrow-right"
                  @click="goNext"
                >
                  Начать настройку
                </UButton>
              </div>
            </section>

            <!-- Шаг 2: ОФО и должность -->
            <section
              v-show="currentStep === 'work'"
              class="flex flex-col gap-5"
              aria-labelledby="onboarding-work-title"
            >
              <div class="text-center space-y-2">
                <h2
                  id="onboarding-work-title"
                  class="text-xl sm:text-2xl font-semibold text-highlighted font-unbounded"
                >
                  Место работы
                </h2>
                <p class="text-sm text-muted max-w-lg mx-auto">
                  Укажите подразделение и должность — они нужны для журнала отсутствия
                  и отображения вас в корпоративных сервисах.
                </p>
              </div>

              <UAlert
                v-if="!ofoAlreadySet"
                class="max-w-md mx-auto w-full"
                color="info"
                variant="subtle"
                icon="i-lucide-info"
                title="Выбирайте самое конкретное подразделение"
              >
                <template #description>
                  <p>Введите в поиске тот отдел, в котором вы работаете, — самый нижний уровень структуры.</p>
                  <dl class="mt-3 flex flex-col gap-2">
                    <div class="rounded-md bg-default/60 px-3 py-2">
                      <dt class="font-medium text-highlighted">Вы сотрудник отдела</dt>
                      <dd class="mt-0.5">
                        Работаете в управлении информационных технологий и цифрового развития, в отделе по
                        информатизации? Введите <b class="font-semibold text-highlighted">«Отдел по информатизации»</b>.
                      </dd>
                    </div>
                    <div class="rounded-md bg-default/60 px-3 py-2">
                      <dt class="font-medium text-highlighted">Вы начальник управления</dt>
                      <dd class="mt-0.5">
                        Введите название управления — например,
                        <b class="font-semibold text-highlighted">«Управление информационных технологий и цифрового развития»</b>.
                      </dd>
                    </div>
                  </dl>
                </template>
              </UAlert>

              <UForm class="space-y-4 max-w-md mx-auto w-full">
                <UserWorkFields
                  v-model:ofo-id="form.ofoId"
                  v-model:role="form.role"
                  :ofo-locked="ofoAlreadySet"
                  required
                  size="xl"
                  ofo-help="Категория — заголовок, раскройте и выберите подразделение."
                />
              </UForm>

              <div class="flex flex-wrap justify-between gap-3 pt-2">
                <UButton
                  size="xl"
                  color="neutral"
                  variant="outline"
                  leading-icon="i-lucide-arrow-left"
                  @click="goBack"
                >
                  Назад
                </UButton>
                <UButton
                  size="xl"
                  trailing-icon="i-lucide-arrow-right"
                  :disabled="!canProceedFromWork"
                  @click="onWorkNext"
                >
                  Далее
                </UButton>
              </div>
            </section>

            <!-- Шаг 3: аватар -->
            <section
              v-show="currentStep === 'avatar'"
              class="flex flex-col gap-5"
              aria-labelledby="onboarding-avatar-title"
            >
              <div class="text-center space-y-2">
                <h2
                  id="onboarding-avatar-title"
                  class="text-xl sm:text-2xl font-semibold text-highlighted font-unbounded"
                >
                  Выберите аватар
                </h2>
                <p class="text-sm text-muted">
                  Аватар отображается в шапке портала и в профиле.
                </p>
              </div>

              <div class="flex flex-col items-center gap-4">
                <UAvatar
                  :src="form.avatarUrl"
                  :alt="greetingName"
                  size="3xl"
                  class="size-24 sm:size-28 ring-4 ring-primary/20"
                  :ui="{ root: '!bg-elevated' }"
                />

                <div
                  class="grid grid-cols-3 sm:grid-cols-5 gap-2 w-full max-w-md"
                  role="listbox"
                  aria-label="Список аватаров"
                >
                  <UButton
                    v-for="name in PROFILE_AVATAR_FILENAMES"
                    :key="name"
                    type="button"
                    size="lg"
                    :variant="form.avatarUrl === avatarUrlFromFilename(name) ? 'solid' : 'subtle'"
                    :color="form.avatarUrl === avatarUrlFromFilename(name) ? 'primary' : 'neutral'"
                    class="aspect-square p-1"
                    :aria-selected="form.avatarUrl === avatarUrlFromFilename(name)"
                    role="option"
                    @click="selectAvatar(name)"
                  >
                    <img
                      :src="avatarUrlFromFilename(name)"
                      alt=""
                      class="w-10 h-auto mx-auto"
                    />
                  </UButton>
                </div>
              </div>

              <div class="flex flex-wrap justify-between gap-3 pt-2">
                <UButton
                  size="xl"
                  color="neutral"
                  variant="outline"
                  leading-icon="i-lucide-arrow-left"
                  @click="goBack"
                >
                  Назад
                </UButton>
                <UButton
                  size="xl"
                  trailing-icon="i-lucide-arrow-right"
                  :disabled="!canProceedFromAvatar"
                  @click="onAvatarNext"
                >
                  Далее
                </UButton>
              </div>
            </section>

            <!-- Шаг 4: внешний вид — минимум текста, всё применяется сразу -->
            <section
              v-show="currentStep === 'look'"
              class="flex flex-col gap-6"
              aria-labelledby="onboarding-look-title"
            >
              <div class="text-center space-y-2">
                <h2
                  id="onboarding-look-title"
                  class="text-xl sm:text-2xl font-semibold text-highlighted font-unbounded"
                >
                  Внешний вид
                </h2>
                <p class="text-sm text-muted">Меняется сразу. Позже — в меню профиля.</p>
              </div>

              <div class="flex flex-col gap-5 max-w-xl mx-auto w-full">
                <!-- Масштаб: своей настройки нет — масштаб браузера -->
                <div class="flex flex-wrap items-center justify-center gap-x-4 gap-y-2 rounded-panel bg-primary/10 px-4 py-3 text-sm text-highlighted">
                  <span class="inline-flex items-center gap-2 font-medium">
                    <UIcon name="i-lucide-zoom-in" class="size-4 text-primary" aria-hidden="true" />
                    Подберите масштаб:
                  </span>
                  <span class="inline-flex items-center gap-1"><UKbd>Ctrl</UKbd><UKbd>+</UKbd></span>
                  <span class="inline-flex items-center gap-1"><UKbd>Ctrl</UKbd><UKbd>−</UKbd></span>
                </div>

                <div class="flex flex-col gap-2">
                  <p class="text-sm font-medium text-highlighted">Тема</p>
                  <div class="flex flex-wrap gap-2" role="radiogroup" aria-label="Тема">
                    <UButton
                      v-for="m in ([{ id: 'light', label: 'Светлая', icon: 'i-lucide-sun' }, { id: 'dark', label: 'Тёмная', icon: 'i-lucide-moon' }] as const)"
                      :key="m.id"
                      type="button"
                      :icon="m.icon"
                      :color="colorMode === m.id ? 'primary' : 'neutral'"
                      :variant="colorMode === m.id ? 'soft' : 'outline'"
                      role="radio"
                      :aria-checked="colorMode === m.id"
                      @click="setColorMode(m.id)"
                    >
                      {{ m.label }}
                    </UButton>
                  </div>
                </div>

                <div class="flex flex-col gap-2">
                  <p class="text-sm font-medium text-highlighted">Цвет</p>
                  <div class="flex flex-wrap gap-2.5 p-1" role="radiogroup" aria-label="Цвет">
                    <button
                      v-for="c in PRIMARY_COLORS"
                      :key="c"
                      type="button"
                      role="radio"
                      :aria-checked="appConfig.ui.colors.primary === c"
                      :aria-label="PRIMARY_COLOR_LABELS[c]"
                      :title="PRIMARY_COLOR_LABELS[c]"
                      class="size-8 rounded-full ring-offset-2 ring-offset-(--ui-bg) transition-shadow focus-visible:outline-2 focus-visible:outline-primary"
                      :class="[
                        c === 'white' ? 'bg-inverted' : 'bg-(--chip-light) dark:bg-(--chip-dark)',
                        appConfig.ui.colors.primary === c ? 'ring-2 ring-highlighted' : 'ring-1 ring-default',
                      ]"
                      :style="{ '--chip-light': `var(--color-${c}-500)`, '--chip-dark': `var(--color-${c}-400)` }"
                      @click="patchAppTheme({ primary: c })"
                    />
                  </div>
                </div>

                <div class="flex flex-col gap-2">
                  <p class="text-sm font-medium text-highlighted">Шрифт</p>
                  <div class="flex flex-wrap gap-2" role="radiogroup" aria-label="Шрифт">
                    <UButton
                      v-for="f in FONT_OPTIONS"
                      :key="f.id"
                      type="button"
                      :color="appConfig.ui.font === f.id ? 'primary' : 'neutral'"
                      :variant="appConfig.ui.font === f.id ? 'soft' : 'outline'"
                      role="radio"
                      :aria-checked="appConfig.ui.font === f.id"
                      :style="{ fontFamily: f.value }"
                      @click="patchAppTheme({ font: f.id })"
                    >
                      {{ f.label }}
                    </UButton>
                  </div>
                </div>

                <div class="flex flex-col gap-2">
                  <p class="text-sm font-medium text-highlighted">Скругление углов</p>
                  <div class="flex flex-wrap gap-2" role="radiogroup" aria-label="Скругление углов">
                    <UButton
                      v-for="r in RADIUS_OPTIONS"
                      :key="r.id"
                      type="button"
                      :color="appConfig.ui.radius === r.id ? 'primary' : 'neutral'"
                      :variant="appConfig.ui.radius === r.id ? 'soft' : 'outline'"
                      role="radio"
                      :aria-checked="appConfig.ui.radius === r.id"
                      @click="patchAppTheme({ radius: r.id })"
                    >
                      {{ r.label }}
                    </UButton>
                  </div>
                </div>
              </div>

              <div class="flex flex-wrap justify-between gap-3 pt-2">
                <UButton
                  size="xl"
                  color="neutral"
                  variant="outline"
                  leading-icon="i-lucide-arrow-left"
                  @click="goBack"
                >
                  Назад
                </UButton>
                <UButton size="xl" trailing-icon="i-lucide-arrow-right" @click="goNext">
                  Далее
                </UButton>
              </div>
            </section>

            <!-- Шаг 5: завершение -->
            <section
              v-show="currentStep === 'done'"
              class="flex flex-col gap-5"
              aria-labelledby="onboarding-done-title"
            >
              <div class="text-center space-y-3">
                <div
                  class="mx-auto flex size-16 items-center justify-center rounded-full bg-primary/15 text-primary"
                >
                  <UIcon name="i-lucide-party-popper" class="size-8" />
                </div>
                <h2
                  id="onboarding-done-title"
                  class="text-xl sm:text-2xl font-semibold text-highlighted font-unbounded"
                >
                  Всё готово!
                </h2>
                <p class="text-sm text-muted max-w-md mx-auto">
                  Проверьте данные перед входом на главную страницу портала. Назначенные вам курсы
                  появятся в разделе «Обучение», а цвета, шрифт и скругление можно поменять в меню профиля.
                </p>
              </div>

              <UCard variant="subtle" class="max-w-md mx-auto w-full ring-1 ring-default">
                <dl class="space-y-3 text-sm">
                  <div class="flex justify-between gap-4">
                    <dt class="text-muted">Подразделение</dt>
                    <dd class="font-medium text-highlighted text-right">
                      {{ selectedOfoLabel || '—' }}
                    </dd>
                  </div>
                  <USeparator />
                  <div class="flex justify-between gap-4">
                    <dt class="text-muted">Должность</dt>
                    <dd class="font-medium text-highlighted text-right">
                      {{ form.role || '—' }}
                    </dd>
                  </div>
                  <USeparator />
                  <div class="flex items-center justify-between gap-4">
                    <dt class="text-muted">Аватар</dt>
                    <dd>
                      <UAvatar
                        :src="form.avatarUrl"
                        size="md"
                        :ui="{ root: '!bg-elevated' }"
                      />
                    </dd>
                  </div>
                  <USeparator />
                  <div class="flex justify-between gap-4">
                    <dt class="text-muted">Оформление</dt>
                    <dd class="font-medium text-highlighted text-right">{{ lookSummary }}</dd>
                  </div>
                </dl>
              </UCard>

              <div class="flex flex-wrap justify-between gap-3 pt-2">
                <UButton
                  size="xl"
                  color="neutral"
                  variant="outline"
                  leading-icon="i-lucide-arrow-left"
                  :disabled="saving"
                  @click="goBack"
                >
                  Назад
                </UButton>
                <UButton
                  size="xl"
                  icon="i-lucide-rocket"
                  :loading="saving"
                  @click="finishOnboarding"
                >
                  Перейти на главную
                </UButton>
              </div>
            </section>
          </template>
        </div>
      </UCard>
    </div>
  </div>
</template>
