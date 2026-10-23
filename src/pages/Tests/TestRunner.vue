<script setup lang="ts">
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue';
import draggable from 'vuedraggable';
import { useTestsStore } from '../../composables/useTestsStore';
import { pairingCorrect, typeIsPairing, type PairingMap, type Question } from './questionTypes';
import { type TestForm } from './testForm';

const props = withDefaults(defineProps<{
  form: TestForm;
  previewHint?: boolean;
  persistSession?: boolean;
  record?: boolean;
  /** Восстановление ответов (например, из course attemptGet) */
  initialAnswers?: Record<string, unknown> | null;
  submit?: (answers: Record<string, unknown>, durationSec: number) => Promise<unknown>;
  /** Показать «Пройти ещё раз» на экране результата */
  allowRetake?: boolean;
}>(), {
  previewHint: false,
  persistSession: false,
  record: false,
  initialAnswers: null,
  submit: undefined,
  allowRetake: true,
});
const emit = defineEmits<{
  (e: 'finish', payload: {
    answers: Record<string, unknown>;
    durationSec: number;
    result?: {
      score: number | null;
      passed: boolean | null;
      correctCount?: number;
      scorable?: number;
    } | null;
  }): void;
  (e: 'retake'): void;
  /** Нажали «Пройти тест» — началось прохождение (страница теста курса заводит новую попытку, если прошлая завершена). */
  (e: 'start'): void;
}>();

const store = useTestsStore();

const kindLabels: Record<string, string> = { test: 'Тест', survey: 'Опрос', poll: 'Голосование' };
const isTest = computed(() => props.form.kind === 'test');
const isPoll = computed(() => props.form.kind === 'poll');
const ctaLabel = computed(() =>
  isPoll.value ? 'Участвовать в голосовании' : props.form.kind === 'survey' ? 'Пройти опрос' : 'Пройти тест',
);
const finishLabel = computed(() => (isPoll.value ? 'Проголосовать' : 'Завершить'));
const finishIcon = computed(() => (isPoll.value ? 'i-lucide-vote' : 'i-lucide-check'));

type ViewItem = { q: Question; options: { id: string; text: string }[] };

function shuffled<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [a[i], a[j]] = [a[j], a[i]]; }
  return a;
}

// ── Навигация ─────────────────────────────────────────────────────────────────
const view = ref<ViewItem[]>([]);
const page = ref(0); // 0 = титул
const total = computed(() => view.value.length + 1);
const currentItem = computed(() => view.value[page.value - 1]);
const currentQuestion = computed(() => currentItem.value?.q);
const currentOptions = computed(() => currentItem.value?.options ?? []);
const isLastPage = computed(() => page.value === view.value.length && view.value.length > 0);

/**
 * «Назад» есть всегда: раньше он зависел от «Свободного перехода», а «Далее»
 * (пропустить вопрос) был всегда — вперёд пропустить можно, вернуться нельзя.
 * Что можно менять при возврате, решает «Разрешить изменять ответ»: без неё
 * данный ответ зафиксирован, а пропущенный вопрос остаётся открытым.
 */
const canGoBack = computed(() => page.value > 1 && !timeUp.value && !revealing.value);

// ── Ответы ───────────────────────────────────────────────────────────────────
const answers = reactive<Record<string, unknown>>({});
const lockedIds = ref<Set<string>>(new Set()); // заблокированные (при выкл. «изменять ответ»)
const currentLocked = computed(() => {
  const q = currentQuestion.value;
  return !!q && (lockedIds.value.has(q.id) || revealing.value || resultsView.value !== null);
});

/** Элементы слева у соответствия / классификации (пустые по тексту не показываем). */
function pairItems(q: Question) {
  return (q.items ?? []).filter((it) => it.text.trim());
}
function pairMap(q: Question): PairingMap {
  const v = answers[q.id];
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as PairingMap) : {};
}

function isAnswered(q: Question): boolean {
  if (typeIsPairing(q.type)) {
    const items = pairItems(q);
    const m = pairMap(q);
    return items.length > 0 && items.every((it) => Boolean(m[it.id]));
  }
  const v = answers[q.id];
  if (q.type === 'multiple') return Array.isArray(v) && v.length > 0;
  if (q.type === 'number' || q.type === 'scale') return v !== undefined && v !== null && (v as unknown) !== '';
  if (typeof v === 'string') return v.trim() !== '';
  return v !== undefined && v !== null;
}
function scaleNumbers(q: Question) {
  const min = Math.min(q.scaleMin, q.scaleMax), max = Math.max(q.scaleMin, q.scaleMax);
  const arr: number[] = [];
  for (let n = min; n <= max && arr.length < 50; n++) arr.push(n);
  return arr;
}

// выбор варианта
function onPick(optId: string) {
  if (currentLocked.value) return;
  const q = currentQuestion.value!;
  if (q.type === 'multiple') {
    const arr = Array.isArray(answers[q.id]) ? [...(answers[q.id] as string[])] : [];
    const i = arr.indexOf(optId);
    if (i >= 0) arr.splice(i, 1); else arr.push(optId);
    answers[q.id] = arr;
  } else {
    answers[q.id] = optId;
  }
}
function isSelected(optId: string): boolean {
  const q = currentQuestion.value!;
  const v = answers[q.id];
  return Array.isArray(v) ? v.includes(optId) : v === optId;
}

// ── Правильные ответы (тесты) ─────────────────────────────────────────────────
function hasCorrect(q: Question): boolean {
  if (typeIsPairing(q.type)) return Object.keys(pairingCorrect(q)).length > 0;
  const c = q.correct;
  if (c == null || c === '') return false;
  return Array.isArray(c) ? c.length > 0 : true;
}
function isCorrectOption(q: Question, optId: string): boolean {
  if (q.type === 'multiple') return Array.isArray(q.correct) && q.correct.includes(optId);
  return q.correct === optId;
}
const shouldRevealImmediate = computed(
  () => isTest.value && props.form.showCorrectAnswers && !props.form.allowChangeAnswer && props.form.showResult === 'immediate',
);
const revealing = ref(false);

function optionRowClass(optId: string, boxed = true): string {
  const q = currentQuestion.value!;
  if (revealing.value && hasCorrect(q)) {
    if (isCorrectOption(q, optId)) return boxed ? 'ring-success bg-success/10 text-success' : 'text-success';
    if (isSelected(optId)) return boxed ? 'ring-error bg-error/10 text-error' : 'text-error';
    return boxed ? 'ring-default' : 'text-default';
  }
  if (isSelected(optId)) return boxed ? 'ring-primary bg-primary/10 text-highlighted' : 'text-highlighted';
  return boxed ? 'ring-default hover:bg-elevated' : 'text-default hover:text-highlighted';
}

// ── Таймер ────────────────────────────────────────────────────────────────────
const startTs = ref<number | null>(null);
const nowTs = ref(Date.now());
const timeUp = ref(false);
let timerId: ReturnType<typeof setInterval> | null = null;
function hmsToSeconds(v: string): number { const [h = 0, m = 0, s = 0] = v.split(':').map(Number); return h * 3600 + m * 60 + s; }
function secondsToHms(t: number): string {
  const x = Math.max(0, Math.floor(t));
  return [Math.floor(x / 3600), Math.floor((x % 3600) / 60), x % 60].map((n) => String(n).padStart(2, '0')).join(':');
}
const elapsed = computed(() => (startTs.value ? Math.floor((nowTs.value - startTs.value) / 1000) : 0));
const timerLabel = computed(() =>
  props.form.useTimeLimit && props.form.timeLimit ? secondsToHms(hmsToSeconds(props.form.timeLimit) - elapsed.value) : secondsToHms(elapsed.value),
);
const timeLow = computed(() => props.form.useTimeLimit && !!props.form.timeLimit && hmsToSeconds(props.form.timeLimit) - elapsed.value <= 60);
function checkTimeUp() {
  if (props.form.useTimeLimit && props.form.timeLimit && startTs.value && hmsToSeconds(props.form.timeLimit) - elapsed.value <= 0) {
    timeUp.value = true; finishOpen.value = false; unansweredOpen.value = false;
    if (timerId) { clearInterval(timerId); timerId = null; }
  }
}
function startInterval() {
  nowTs.value = Date.now();
  if (timerId) clearInterval(timerId);
  timerId = setInterval(() => { nowTs.value = Date.now(); checkTimeUp(); }, 1000);
  checkTimeUp();
}
function stopTimer() { if (timerId) { clearInterval(timerId); timerId = null; } }
onUnmounted(stopTimer);

// ── Сессия (для «Продолжить») ─────────────────────────────────────────────────
function saveSessionNow() {
  if (!props.persistSession || props.form.id == null || page.value < 1 || !startTs.value) return;
  store.saveSession(props.form.id, { page: page.value, answers: { ...answers }, startTs: startTs.value });
}
watch(page, saveSessionNow);
watch(answers, saveSessionNow, { deep: true });

function buildView() {
  const base: ViewItem[] = props.form.questions.map((q) => ({
    q,
    options: (props.form.shuffleOptions || q.type === 'match') ? shuffled(q.options ?? []).map((o) => ({ id: o.id, text: o.text })) : (q.options ?? []).map((o) => ({ id: o.id, text: o.text })),
  }));
  view.value = props.form.shuffle && !isPoll.value ? shuffled(base) : base;
}

function initSession() {
  if (props.persistSession && props.form.id != null) {
    const s = store.getSession(props.form.id);
    if (s && store.hasActiveSession(props.form)) {
      buildView();
      Object.assign(answers, s.answers);
      startTs.value = s.startTs;
      page.value = s.page;
      startInterval();
      return;
    }
  }
  page.value = 0;
}
initSession();

// ── Старт / навигация ─────────────────────────────────────────────────────────
function start() {
  if (!props.form.questions.length) return;
  for (const k in answers) delete answers[k];
  if (props.initialAnswers) Object.assign(answers, props.initialAnswers);
  lockedIds.value = new Set();
  revealing.value = false;
  resultsView.value = null;
  submitError.value = null;
  timeUp.value = false;
  buildView();
  page.value = 1;
  startTs.value = Date.now();
  startInterval();
  saveSessionNow();
  emit('start');
}
/** Фиксируем только данный ответ: пропущенный через «Далее» вопрос должен остаться открытым. */
function lockCurrentIfNeeded() {
  const q = currentQuestion.value;
  if (q && !props.form.allowChangeAnswer && isAnswered(q)) lockedIds.value.add(q.id);
}
function advance() { if (page.value < total.value - 1) { lockCurrentIfNeeded(); page.value++; } }
function prev() { if (canGoBack.value) page.value--; }

// показ правильных «сразу»: 0.7с подсветки перед переходом
function withReveal(action: () => void) {
  if (timeUp.value || !shouldRevealImmediate.value || !currentQuestion.value || !hasCorrect(currentQuestion.value) || revealing.value) {
    action();
    return;
  }
  revealing.value = true;
  setTimeout(() => { revealing.value = false; action(); }, 700);
}
// «Ответить» — подтверждает ответ (с подсветкой правильных) и ведёт дальше
function onAnswer() { withReveal(advance); }

// ── Завершение ────────────────────────────────────────────────────────────────
const finishOpen = ref(false);
const unansweredOpen = ref(false);
const reviewActive = ref(false); // прогон по неотвеченным
const resultsView = ref<null | 'review' | 'poll' | 'score'>(null); // финальные экраны
const pollResults = ref<Record<string, { count: number; percent: number }>>({});
let lastDuration = 0;
const submitting = ref(false);
const submitError = ref<string | null>(null);

// Соответствие / классификация: карточки перетаскиваются между зонами.
// zones: 'pool' — ещё не разложенные; у classify остальные ключи — id категорий (в них утверждения),
// у match — id понятий (в каждой не больше одного определения). Ответ собираем из zones.
type Card = { id: string; text: string };
const POOL = 'pool';
const zones = reactive<Record<string, Card[]>>({ [POOL]: [] });
// Клавиатура: Enter/Пробел на карточке из общего набора выбирает её, затем «Положить сюда» в нужной зоне.
const picked = ref<string | null>(null);
const pickedCard = computed(() => (picked.value ? zones[POOL]?.find((c) => c.id === picked.value) ?? null : null));

function buildZones() {
  picked.value = null;
  const q = currentQuestion.value;
  if (!q || !typeIsPairing(q.type)) return;
  const m = pairMap(q);
  for (const k of Object.keys(zones)) delete zones[k];
  const items = pairItems(q);
  if (q.type === 'classify') {
    zones[POOL] = items.filter((i) => !m[i.id]).map((i) => ({ id: i.id, text: i.text }));
    for (const o of currentOptions.value) {
      zones[o.id] = items.filter((i) => m[i.id] === o.id).map((i) => ({ id: i.id, text: i.text }));
    }
  } else {
    const used = new Set(Object.values(m));
    zones[POOL] = currentOptions.value.filter((o) => !used.has(o.id)).map((o) => ({ id: o.id, text: o.text }));
    for (const it of items) {
      const o = currentOptions.value.find((x) => x.id === m[it.id]);
      zones[it.id] = o ? [{ id: o.id, text: o.text }] : [];
    }
  }
}
watch([() => currentQuestion.value?.id, page, resultsView], buildZones, { immediate: true });

function commitZones() {
  const q = currentQuestion.value;
  if (!q) return;
  const next: PairingMap = {};
  if (q.type === 'classify') {
    for (const o of currentOptions.value) for (const c of zones[o.id] ?? []) next[c.id] = o.id;
  } else {
    for (const it of pairItems(q)) {
      const c = zones[it.id]?.[0];
      if (c) next[it.id] = c.id;
    }
  }
  answers[q.id] = next;
}
function onZoneChange(zoneKey: string, evt: { added?: { element: Card } }) {
  const q = currentQuestion.value;
  if (q?.type === 'match' && zoneKey !== POOL && (zones[zoneKey]?.length ?? 0) > 1) {
    // В ячейке одно определение: новое остаётся, прежнее возвращается к остальным.
    const keep = evt.added?.element ?? zones[zoneKey][zones[zoneKey].length - 1];
    zones[POOL].push(...zones[zoneKey].filter((c) => c.id !== keep.id));
    zones[zoneKey] = [keep];
  }
  commitZones();
}
function focusCard(zoneKey: string, cardId: string) {
  nextTick(() => {
    const key = `${zoneKey}|${cardId}`;
    ([...document.querySelectorAll<HTMLElement>('[data-card-key]')].find((e) => e.dataset.cardKey === key))?.focus();
  });
}
/** Нажатие на разложенную карточку возвращает её к остальным. */
function returnToPool(zoneKey: string, cardId: string) {
  if (currentLocked.value) return;
  const card = zones[zoneKey]?.find((c) => c.id === cardId);
  if (!card) return;
  zones[zoneKey] = zones[zoneKey].filter((c) => c.id !== cardId);
  zones[POOL].push(card);
  commitZones();
  focusCard(POOL, cardId);
}
function togglePick(cardId: string) {
  if (currentLocked.value) return;
  picked.value = picked.value === cardId ? null : cardId;
}
/** Выбранную с клавиатуры карточку кладём в зону — как при перетаскивании. */
function placePicked(zoneKey: string) {
  const card = pickedCard.value;
  if (!card || currentLocked.value) return;
  zones[POOL] = zones[POOL].filter((c) => c.id !== card.id);
  zones[zoneKey] = [...(zones[zoneKey] ?? []), card];
  picked.value = null;
  onZoneChange(zoneKey, { added: { element: card } });
  focusCard(zoneKey, card.id);
}
function cardClass(cardId: string, zoneKey: string): string {
  const base = 'rounded-lg ring-1 px-3 py-2 text-sm flex items-center gap-2 select-none transition-colors ';
  const q = currentQuestion.value;
  const interactive = (currentLocked.value ? 'cursor-default' : 'cursor-grab active:cursor-grabbing')
    + ' focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary'
    + (zoneKey === POOL && picked.value === cardId ? ' ring-2 ring-primary' : '');
  if (q && revealing.value && zoneKey !== POOL) {
    const key = pairingCorrect(q);
    const ok = q.type === 'classify' ? key[cardId] === zoneKey : key[zoneKey] === cardId;
    return base + interactive + (ok ? ' ring-success bg-success/10 text-success' : ' ring-error bg-error/10 text-error');
  }
  return base + interactive + (zoneKey === POOL ? ' ring-default bg-default text-default hover:bg-elevated' : ' ring-primary bg-primary/10 text-highlighted');
}


const unanswered = computed(() => view.value.map((it, i) => ({ q: it.q, page: i + 1 })).filter((x) => !isAnswered(x.q)));
const hasRequiredUnanswered = computed(() => unanswered.value.some((x) => x.q.required));
const nextUnansweredPage = computed(() => unanswered.value.find((x) => x.page > page.value)?.page ?? null);
const unansweredLabel = computed(() => (unanswered.value.length === 1 ? 'Перейти к вопросу' : 'Перейти к вопросам'));
function plural(n: number, one: string, few: string, many: string): string {
  const m10 = n % 10, m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20)) return few;
  return many;
}

// Подтверждение нужно только если ответы можно было менять
// (тест/опрос — «изменять ответ»; голосование — когда нельзя переголосовать)
const needsFinishConfirm = computed(() => (isPoll.value ? !props.form.allowRevote : props.form.allowChangeAnswer));

function onFinishClick() { withReveal(attemptFinish); }
function attemptFinish() {
  if (unanswered.value.length > 0) { unansweredOpen.value = true; return; }
  if (needsFinishConfirm.value) finishOpen.value = true;
  else finalize();
}
function goToUnanswered() {
  unansweredOpen.value = false;
  reviewActive.value = true;
  // Страховка: вопрос без ответа не может быть зафиксирован (например, из старой сессии).
  for (const x of unanswered.value) lockedIds.value.delete(x.q.id);
  const first = unanswered.value[0];
  if (first) page.value = first.page;
}

// Результат теста (по правильным ответам на клиенте или с сервера)
const clientScoreInfo = computed(() => {
  let scorable = 0, correct = 0;
  for (const it of view.value) {
    if (!hasCorrect(it.q)) continue;
    scorable++;
    if (answeredCorrectly(it)) correct++;
  }
  return { scorable, correct, percent: scorable > 0 ? Math.round((correct / scorable) * 100) : 0 };
});
const serverResult = ref<{
  score: number | null;
  passed: boolean | null;
  correctCount: number;
  scorable: number;
} | null>(null);

const scoreInfo = computed(() => {
  if (serverResult.value && (serverResult.value.scorable > 0 || serverResult.value.score != null)) {
    const scorable = serverResult.value.scorable || 0;
    const correct = serverResult.value.correctCount || 0;
    const percent = serverResult.value.score != null
      ? Math.round(Number(serverResult.value.score))
      : (scorable > 0 ? Math.round((correct / scorable) * 100) : 0);
    return { scorable, correct, percent };
  }
  return clientScoreInfo.value;
});
const passed = computed(() => {
  if (serverResult.value && serverResult.value.passed !== undefined) {
    return serverResult.value.passed;
  }
  return props.form.usePassingScore ? scoreInfo.value.percent >= props.form.passingScore : null;
});

async function finalize() {
  if (submitting.value) return;
  stopTimer();
  finishOpen.value = false;
  unansweredOpen.value = false;
  submitError.value = null;
  lastDuration = startTs.value ? Math.floor((Date.now() - startTs.value) / 1000) : 0;

  let recorded = false;
  serverResult.value = null;
  submitting.value = true;
  try {
    if (props.submit) {
      const raw = await props.submit({ ...answers }, lastDuration) as any;
      recorded = true;
      if (raw && typeof raw === 'object' && ('score' in raw || 'passed' in raw || 'scorable' in raw)) {
        serverResult.value = {
          score: raw.score != null ? Number(raw.score) : null,
          passed: raw.passed === true ? true : raw.passed === false ? false : null,
          correctCount: Number(raw.correctCount ?? 0),
          scorable: Number(raw.scorable ?? 0),
        };
      }
    } else if (props.record && props.form.id != null) {
      await store.submitAttempt(props.form.id, { ...answers }, lastDuration);
      recorded = true;
    }
  } catch (e) {
    if (props.submit) {
      // Ответы не дошли до сервера: результата нет, «0» показывать нельзя. Остаёмся на вопросе —
      // ответы и сессия сохранены, можно повторить. На сервере попытка остаётся «в процессе».
      submitError.value = (e as Error)?.message || 'Не удалось отправить ответы';
      startInterval();
      return;
    }
    recorded = false; /* напр. лимит попыток */
  } finally {
    submitting.value = false;
  }
  if (props.persistSession && props.form.id != null) store.clearSession(props.form.id);

  // Живые результаты голосования
  if (isPoll.value && props.form.liveResults && recorded) {
    try {
      const stats = await store.loadStats(props.form.id!);
      const q = (stats?.questions ?? []).find((x: any) => x.id === currentQuestion.value?.id) ?? stats?.questions?.[0];
      const map: Record<string, { count: number; percent: number }> = {};
      for (const o of (q?.options ?? [])) if (o.id) map[o.id] = { count: o.count, percent: o.percent };
      pollResults.value = map;
      resultsView.value = 'poll';
      return;
    } catch { /* без живых — просто завершаем */ }
  }

  // Результат теста: клиентский разбор или серверный балл (когда correct снят с формы)
  const canShowScore = scoreInfo.value.scorable > 0
    || (serverResult.value != null && (serverResult.value.scorable > 0 || serverResult.value.score != null));
  if (isTest.value && props.form.showResult !== 'never' && canShowScore) {
    const canReview = props.form.showCorrectAnswers
      && props.form.showResult === 'after'
      && clientScoreInfo.value.scorable > 0;
    resultsView.value = canReview ? 'review' : 'score';
    return;
  }

  emitFinish();
}
function emitFinish() {
  emit('finish', {
    answers: { ...answers },
    durationSec: lastDuration,
    result: serverResult.value
      ? {
          score: serverResult.value.score,
          passed: serverResult.value.passed,
          correctCount: serverResult.value.correctCount,
          scorable: serverResult.value.scorable,
        }
      : {
          score: scoreInfo.value.percent,
          passed: passed.value,
          correctCount: scoreInfo.value.correct,
          scorable: scoreInfo.value.scorable,
        },
  });
  page.value = 0;
  resultsView.value = null;
  reviewActive.value = false;
  serverResult.value = null;
}

function onRetake() {
  if (props.submit) {
    emit('retake');
    return;
  }
  serverResult.value = null;
  reviewActive.value = false;
  stopTimer();
  start();
}

// форматирование для экрана разбора
function optText(it: ViewItem, id: string): string {
  return it.options.find((o) => o.id === id)?.text || '—';
}
function pairLines(it: ViewItem, map: PairingMap): string {
  return pairItems(it.q)
    .map((item) => `${item.text} → ${it.options.find((o) => o.id === map[item.id])?.text ?? '—'}`)
    .join('\n');
}
function userAnswerText(it: ViewItem): string {
  const q = it.q; const v = answers[q.id];
  if (typeIsPairing(q.type)) return Object.keys(pairMap(q)).length ? pairLines(it, pairMap(q)) : '— нет ответа';
  if (v == null || v === '' || (Array.isArray(v) && !v.length)) return '— нет ответа';
  if (q.type === 'single' || q.type === 'dropdown') return optText(it, v as string);
  if (q.type === 'multiple') return (v as string[]).map((id) => optText(it, id)).join(', ');
  if (q.type === 'yesno') return v === 'yes' ? 'Да' : 'Нет';
  return String(v);
}
function correctAnswerText(it: ViewItem): string {
  const q = it.q; const c = q.correct;
  if (typeIsPairing(q.type)) return pairLines(it, pairingCorrect(q));
  if (c == null || c === '') return '—';
  if (q.type === 'single' || q.type === 'dropdown') return optText(it, c as string);
  if (q.type === 'multiple') return (c as string[]).map((id) => optText(it, id)).join(', ');
  if (q.type === 'yesno') return c === 'yes' ? 'Да' : 'Нет';
  return String(c);
}
function answeredCorrectly(it: ViewItem): boolean {
  const q = it.q;
  if (!hasCorrect(q)) return false;
  if (typeIsPairing(q.type)) {
    const key = pairingCorrect(q);
    const m = pairMap(q);
    return pairItems(q).every((item) => !key[item.id] || m[item.id] === key[item.id]);
  }
  const v = answers[q.id];
  if (q.type === 'multiple') {
    const a = Array.isArray(v) ? [...(v as string[])].sort() : [];
    const c = Array.isArray(q.correct) ? [...q.correct].sort() : [];
    return a.length === c.length && a.every((x, i) => x === c[i]);
  }
  if (q.type === 'text' || q.type === 'textarea' || q.type === 'date') {
    return String(v ?? '').trim().toLowerCase() === String(q.correct).trim().toLowerCase();
  }
  return String(v ?? '') === String(q.correct);
}

function pollPercent(id: string): number { return pollResults.value[id]?.percent ?? 0; }
</script>

<template>
  <div class="flex flex-col w-full min-h-full justify-center">
    <p v-if="previewHint" class="text-xs text-dimmed mb-2 flex items-center justify-center gap-1.5 shrink-0">
      <UIcon name="i-lucide-eye" class="size-3.5" />
      Так тест увидит сотрудник
    </p>

    <!-- Окно растёт по высоте содержимого (без внутренней прокрутки); скроллится страница. -->
    <div class="w-full flex justify-center">
      <div class="w-full max-w-4xl min-h-[26rem] rounded-2xl ring-1 ring-default bg-default shadow-2xl flex flex-col">

        <div v-if="form.showProgress && page > 0 && !resultsView" class="h-1 bg-elevated shrink-0">
          <div class="h-full bg-primary transition-all" :style="{ width: `${(page / Math.max(view.length, 1)) * 100}%` }" />
        </div>

        <div class="flex-1 flex flex-col">
          <!-- Титульник -->
          <div v-if="page === 0 && !resultsView" class="flex-1 flex flex-col items-center justify-center text-center gap-4 px-10 py-6">
            <UBadge color="primary" variant="subtle" size="lg">{{ kindLabels[form.kind] }}</UBadge>
            <h2 class="text-3xl font-semibold text-highlighted leading-tight line-clamp-2">{{ form.title || 'Без названия' }}</h2>
            <p v-if="form.description" class="text-muted max-w-xl line-clamp-4 whitespace-pre-line">{{ form.description }}</p>
            <div class="flex flex-wrap gap-1.5 justify-center">
              <UBadge v-if="form.anonymous" color="neutral" variant="subtle">Анонимно</UBadge>
              <UBadge v-if="form.useTimeLimit && form.timeLimit" color="neutral" variant="subtle">⏱ {{ form.timeLimit }}</UBadge>
              <UBadge color="neutral" variant="subtle">{{ isPoll ? 'Кандидатов' : 'Вопросов' }}: {{ isPoll ? (form.questions[0]?.options.length ?? 0) : form.questions.length }}</UBadge>
            </div>
            <UButton size="xl" class="mt-2" trailing-icon="i-lucide-arrow-right" :disabled="!form.questions.length" @click="start">{{ ctaLabel }}</UButton>
            <p v-if="!form.questions.length" class="text-xs text-dimmed">В форме нет вопросов.</p>
          </div>

          <!-- Финал: разбор теста -->
          <div v-else-if="resultsView === 'review'" class="flex-1 flex flex-col px-8 py-6 gap-3">
            <div class="flex items-center justify-between gap-3 shrink-0">
              <p class="text-lg font-semibold text-highlighted">Ваши ответы</p>
              <UBadge :color="passed === false ? 'error' : 'success'" variant="subtle" size="lg" class="tabular-nums">{{ scoreInfo.correct }}/{{ scoreInfo.scorable }} · {{ scoreInfo.percent }}%</UBadge>
            </div>
            <div class="flex flex-col gap-3 px-1 py-1">
              <div v-for="(it, i) in view" :key="it.q.id" class="rounded-xl ring-1 p-3 flex flex-col gap-1" :class="answeredCorrectly(it) ? 'ring-success/40 bg-success/5' : 'ring-error/40 bg-error/5'">
                <div class="flex items-center gap-2">
                  <UIcon :name="answeredCorrectly(it) ? 'i-lucide-check-circle-2' : 'i-lucide-x-circle'" :class="answeredCorrectly(it) ? 'text-success' : 'text-error'" class="size-4 shrink-0" />
                  <span class="text-sm text-highlighted">{{ i + 1 }}. {{ it.q.title || 'Без названия' }}</span>
                </div>
                <p class="text-sm text-muted pl-6 whitespace-pre-line">Ваш ответ: {{ userAnswerText(it) }}</p>
                <p v-if="!answeredCorrectly(it) && hasCorrect(it.q)" class="text-sm text-success pl-6 whitespace-pre-line">Правильно: {{ correctAnswerText(it) }}</p>
              </div>
            </div>
          </div>

          <!-- Финал: результат теста (процент) -->
          <div v-else-if="resultsView === 'score'" class="flex-1 flex flex-col items-center justify-center text-center gap-4 px-10 py-8">
            <div class="size-24 rounded-full flex items-center justify-center ring-4" :class="passed === false ? 'ring-error/30 bg-error/10' : 'ring-success/30 bg-success/10'">
              <span class="text-3xl font-bold tabular-nums" :class="passed === false ? 'text-error' : 'text-success'">{{ scoreInfo.percent }}%</span>
            </div>
            <p v-if="scoreInfo.scorable > 0" class="text-lg font-semibold text-highlighted">
              Правильных ответов: {{ scoreInfo.correct }} из {{ scoreInfo.scorable }}
            </p>
            <p v-else class="text-lg font-semibold text-highlighted">Ваш результат</p>
            <UBadge v-if="passed !== null" :color="passed ? 'success' : 'error'" variant="subtle" size="lg">
              {{ passed ? 'Тест пройден' : 'Тест не пройден' }} · порог {{ form.passingScore }}%
            </UBadge>
            <p v-if="passed === false" class="text-sm text-muted max-w-sm">
              Результат ниже проходного балла. Закройте экран и попробуйте пройти тест снова.
            </p>
          </div>

          <!-- Страница вопроса -->
          <div v-else-if="currentQuestion" class="flex-1 flex flex-col px-10 py-6">
            <div class="flex items-center justify-between gap-3 shrink-0">
              <p class="text-sm text-dimmed tabular-nums">{{ isPoll ? 'Голосование' : `Вопрос ${page} из ${view.length}` }}</p>
              <div class="flex items-center gap-1.5 text-sm tabular-nums" :class="timeUp || timeLow ? 'text-error font-medium' : 'text-dimmed'">
                <UIcon name="i-lucide-timer" class="size-4" />
                {{ timerLabel }}
              </div>
            </div>

            <div class="flex-1 flex flex-col justify-center gap-5 px-1 py-1">
              <div class="flex flex-col gap-2">
                <h3 class="text-2xl font-medium text-highlighted">
                  {{ currentQuestion.title || 'Без названия' }}
                  <span v-if="currentQuestion.required" class="text-error">*</span>
                </h3>
                <p v-if="currentQuestion.hint" class="text-muted">{{ currentQuestion.hint }}</p>
                <p v-if="currentLocked && !revealing && resultsView === null" class="text-xs text-dimmed">Ответ зафиксирован — изменять нельзя.</p>
              </div>

              <!-- Варианты: single / multiple / poll — кликабельные поля -->
              <div v-if="currentQuestion.type === 'single' || currentQuestion.type === 'multiple'" class="flex flex-col gap-2 w-full p-px" :class="isPoll ? 'max-w-xl' : 'max-w-lg'">
                <button
                  v-for="opt in currentOptions"
                  :key="opt.id"
                  type="button"
                  :disabled="currentLocked"
                  class="relative overflow-hidden flex items-center gap-3 w-full text-left rounded-xl ring-1 transition-colors disabled:cursor-default"
                  :class="[optionRowClass(opt.id, true), isPoll ? 'px-4 py-3.5' : 'px-4 py-2.5']"
                  @click="onPick(opt.id)"
                >
                  <span v-if="isPoll && resultsView === 'poll'" class="absolute inset-y-0 left-0 bg-primary/15 transition-all" :style="{ width: `${pollPercent(opt.id)}%` }" />
                  <span class="relative z-10 flex items-center gap-3 w-full">
                    <span class="shrink-0 inline-flex items-center justify-center size-5" :class="currentQuestion.type === 'multiple' ? 'rounded' : 'rounded-full'">
                      <span class="size-5 ring-1 ring-current flex items-center justify-center" :class="[currentQuestion.type === 'multiple' ? 'rounded' : 'rounded-full', isSelected(opt.id) ? 'text-primary' : 'text-dimmed']">
                        <span v-if="isSelected(opt.id)" class="size-2.5 bg-primary" :class="currentQuestion.type === 'multiple' ? 'rounded-sm' : 'rounded-full'" />
                      </span>
                    </span>
                    <span class="flex-1">{{ opt.text }}</span>
                    <span v-if="isPoll && resultsView === 'poll'" class="text-sm tabular-nums text-muted shrink-0">{{ pollPercent(opt.id) }}%</span>
                    <UIcon v-else-if="revealing && isCorrectOption(currentQuestion, opt.id)" name="i-lucide-check" class="size-4 text-success shrink-0" />
                    <UIcon v-else-if="revealing && isSelected(opt.id)" name="i-lucide-x" class="size-4 text-error shrink-0" />
                  </span>
                </button>
              </div>

              <!-- Соответствие / классификация: карточки перетаскиваются -->
              <div v-else-if="currentQuestion.type === 'match' || currentQuestion.type === 'classify'" class="flex flex-col gap-4 w-full">
                <p class="text-xs text-dimmed">
                  {{ currentQuestion.type === 'match'
                    ? 'Перетащите определения к подходящим понятиям.'
                    : 'Перетащите утверждения в подходящие категории.' }}
                  Ошиблись — перетащите заново или нажмите на карточку, чтобы вернуть её.
                  С клавиатуры: Enter на карточке выбирает её, затем нажмите «Положить сюда» в нужной ячейке.
                </p>
                <p class="sr-only" aria-live="polite">
                  {{ pickedCard ? `Выбрана карточка: ${pickedCard.text}. Перейдите к нужной ячейке и нажмите «Положить сюда».` : '' }}
                </p>

                <!-- Карточки, которые ещё не разложены -->
                <draggable
                  v-model="zones[POOL]"
                  :group="{ name: 'pairing' }"
                  item-key="id"
                  :disabled="currentLocked"
                  ghost-class="opacity-40"
                  :delay="120"
                  :delay-on-touch-only="true"
                  class="flex flex-wrap gap-2 min-h-14 rounded-xl border border-dashed border-default p-2"
                  @change="onZoneChange(POOL, $event)"
                >
                  <template #item="{ element }">
                    <div
                      :class="cardClass(element.id, POOL)"
                      :data-card-key="`${POOL}|${element.id}`"
                      :role="currentLocked ? undefined : 'button'"
                      :tabindex="currentLocked ? undefined : 0"
                      :aria-pressed="currentLocked ? undefined : picked === element.id"
                      @keydown.enter.prevent="togglePick(element.id)"
                      @keydown.space.prevent="togglePick(element.id)"
                    >
                      <UIcon name="i-lucide-grip-vertical" class="size-4 shrink-0 text-dimmed" aria-hidden="true" />
                      <span>{{ element.text }}</span>
                    </div>
                  </template>
                  <template #footer>
                    <p v-if="!zones[POOL]?.length" class="text-xs text-dimmed self-center px-1">Все карточки разложены</p>
                  </template>
                </draggable>

                <!-- Соответствие: строка «понятие → ячейка для определения» -->
                <div v-if="currentQuestion.type === 'match'" class="flex flex-col gap-2">
                  <div v-for="pi in pairItems(currentQuestion)" :key="pi.id" class="grid grid-cols-1 sm:grid-cols-[2fr_3fr] gap-2 items-stretch">
                    <div class="rounded-xl bg-elevated px-4 py-3 text-sm font-medium text-highlighted flex items-center">{{ pi.text }}</div>
                    <draggable
                      v-model="zones[pi.id]"
                      :group="{ name: 'pairing' }"
                      item-key="id"
                      :disabled="currentLocked"
                      ghost-class="opacity-40"
                      :delay="120"
                      :delay-on-touch-only="true"
                      class="min-h-14 rounded-xl border border-dashed border-default p-2 flex flex-col gap-2 justify-center transition-colors"
                      :class="zones[pi.id]?.length ? 'border-solid' : ''"
                      @change="onZoneChange(pi.id, $event)"
                    >
                      <template #item="{ element }">
                        <div
                          :class="cardClass(element.id, pi.id)"
                          :data-card-key="`${pi.id}|${element.id}`"
                          :role="currentLocked ? undefined : 'button'"
                          :tabindex="currentLocked ? undefined : 0"
                          :aria-label="currentLocked ? undefined : `Вернуть карточку: ${element.text}`"
                          @click="returnToPool(pi.id, element.id)"
                          @keydown.enter.prevent="returnToPool(pi.id, element.id)"
                          @keydown.space.prevent="returnToPool(pi.id, element.id)"
                        >
                          <UIcon name="i-lucide-grip-vertical" class="size-4 shrink-0 text-dimmed" aria-hidden="true" />
                          <span>{{ element.text }}</span>
                        </div>
                      </template>
                      <template #footer>
                        <p v-if="!zones[pi.id]?.length" class="text-xs text-dimmed text-center">Перетащите сюда</p>
                        <UButton
                          v-if="pickedCard"
                          type="button"
                          size="xs"
                          color="primary"
                          variant="subtle"
                          icon="i-lucide-corner-down-left"
                          class="self-center"
                          :aria-label="`Положить «${pickedCard.text}» к понятию «${pi.text}»`"
                          @click="placePicked(pi.id)"
                        >
                          Положить сюда
                        </UButton>
                      </template>
                    </draggable>
                  </div>
                </div>

                <!-- Классификация: колонки-категории -->
                <div v-else class="grid grid-cols-1 gap-3" :class="currentOptions.length >= 3 ? 'md:grid-cols-3' : 'md:grid-cols-2'">
                  <div v-for="cat in currentOptions" :key="cat.id" class="flex flex-col gap-2 rounded-xl bg-elevated p-3">
                    <p class="text-sm font-semibold text-highlighted">{{ cat.text }}</p>
                    <draggable
                      v-model="zones[cat.id]"
                      :group="{ name: 'pairing' }"
                      item-key="id"
                      :disabled="currentLocked"
                      ghost-class="opacity-40"
                      :delay="120"
                      :delay-on-touch-only="true"
                      class="min-h-20 flex-1 rounded-lg border border-dashed border-default p-2 flex flex-col gap-2"
                      @change="onZoneChange(cat.id, $event)"
                    >
                      <template #item="{ element }">
                        <div
                          :class="cardClass(element.id, cat.id)"
                          :data-card-key="`${cat.id}|${element.id}`"
                          :role="currentLocked ? undefined : 'button'"
                          :tabindex="currentLocked ? undefined : 0"
                          :aria-label="currentLocked ? undefined : `Вернуть карточку: ${element.text}`"
                          @click="returnToPool(cat.id, element.id)"
                          @keydown.enter.prevent="returnToPool(cat.id, element.id)"
                          @keydown.space.prevent="returnToPool(cat.id, element.id)"
                        >
                          <UIcon name="i-lucide-grip-vertical" class="size-4 shrink-0 text-dimmed" aria-hidden="true" />
                          <span>{{ element.text }}</span>
                        </div>
                      </template>
                      <template #footer>
                        <p v-if="!zones[cat.id]?.length" class="text-xs text-dimmed text-center my-auto">Перетащите сюда</p>
                        <UButton
                          v-if="pickedCard"
                          type="button"
                          size="xs"
                          color="primary"
                          variant="subtle"
                          icon="i-lucide-corner-down-left"
                          class="self-center"
                          :aria-label="`Положить «${pickedCard.text}» в категорию «${cat.text}»`"
                          @click="placePicked(cat.id)"
                        >
                          Положить сюда
                        </UButton>
                      </template>
                    </draggable>
                  </div>
                </div>
              </div>

              <!-- Выпадающий список -->
              <div v-else-if="currentQuestion.type === 'dropdown'" class="w-full max-w-sm flex flex-col gap-1.5">
                <USelect v-model="answers[currentQuestion.id]" :items="currentOptions" value-key="id" label-key="text" :disabled="currentLocked" size="lg" class="w-full" placeholder="Выберите вариант" />
                <p v-if="revealing && hasCorrect(currentQuestion)" class="text-xs text-success">Правильно: {{ correctAnswerText({ q: currentQuestion, options: currentOptions }) }}</p>
              </div>

              <!-- Да / Нет -->
              <div v-else-if="currentQuestion.type === 'yesno'" class="flex gap-3">
                <button
                  v-for="it in [{ id: 'yes', text: 'Да' }, { id: 'no', text: 'Нет' }]"
                  :key="it.id"
                  type="button"
                  :disabled="currentLocked"
                  class="rounded-xl ring-1 px-6 py-2.5 text-sm transition-colors disabled:cursor-default"
                  :class="optionRowClass(it.id)"
                  @click="onPick(it.id)"
                >
                  {{ it.text }}
                </button>
              </div>

              <!-- Шкала -->
              <div v-else-if="currentQuestion.type === 'scale'" class="flex flex-col gap-2 max-w-lg">
                <div class="flex flex-wrap gap-2">
                  <button
                    v-for="n in scaleNumbers(currentQuestion)"
                    :key="n"
                    type="button"
                    :disabled="currentLocked"
                    class="size-10 rounded-lg ring-1 text-sm font-medium transition-colors disabled:cursor-default"
                    :class="revealing && hasCorrect(currentQuestion)
                      ? (String(currentQuestion.correct) === String(n) ? 'ring-success bg-success/10 text-success' : (answers[currentQuestion.id] === n ? 'ring-error bg-error/10 text-error' : 'ring-default text-muted'))
                      : (answers[currentQuestion.id] === n ? 'bg-primary text-inverted ring-primary' : 'ring-default text-muted hover:bg-elevated')"
                    @click="!currentLocked && (answers[currentQuestion.id] = n)"
                  >
                    {{ n }}
                  </button>
                </div>
                <div v-if="currentQuestion.scaleMinLabel || currentQuestion.scaleMaxLabel" class="flex justify-between text-xs text-dimmed">
                  <span>{{ currentQuestion.scaleMinLabel }}</span>
                  <span>{{ currentQuestion.scaleMaxLabel }}</span>
                </div>
              </div>

              <!-- Свободный ответ -->
              <div v-else class="w-full max-w-lg flex flex-col gap-1.5">
                <UInput v-if="currentQuestion.type === 'text'" v-model="answers[currentQuestion.id]" :disabled="currentLocked" size="lg" class="w-full" placeholder="Короткий ответ" />
                <UTextarea v-else-if="currentQuestion.type === 'textarea'" v-model="answers[currentQuestion.id]" :disabled="currentLocked" :rows="3" size="lg" class="w-full" placeholder="Развёрнутый ответ" />
                <UInput v-else-if="currentQuestion.type === 'number'" v-model="answers[currentQuestion.id]" :disabled="currentLocked" type="number" size="lg" class="w-44" placeholder="0" />
                <UInput v-else-if="currentQuestion.type === 'date'" v-model="answers[currentQuestion.id]" :disabled="currentLocked" type="date" size="lg" class="w-52" />
                <p v-if="revealing && hasCorrect(currentQuestion)" class="text-xs text-success">Правильно: {{ correctAnswerText({ q: currentQuestion, options: currentOptions }) }}</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Ошибка отправки -->
        <div v-if="submitError && !resultsView" class="shrink-0 px-5 pt-2">
          <UAlert
            color="error"
            variant="subtle"
            icon="i-lucide-circle-alert"
            title="Ответы не отправлены"
            :description="`${submitError}. Ваши ответы сохранены — нажмите «${finishLabel}», чтобы повторить.`"
          />
        </div>

        <!-- Тайм-аут -->
        <div v-if="page > 0 && timeUp && !resultsView" class="shrink-0 px-5 pt-2 text-center text-xs text-error font-medium">
          Время вышло — ответ на текущий вопрос можно дать, затем форма закроется
        </div>

        <!-- Пейджер финального экрана -->
        <div v-if="resultsView" class="shrink-0 border-t border-default px-5 py-3 flex items-center justify-end gap-2 flex-wrap">
          <UButton
            v-if="allowRetake && (resultsView === 'score' || resultsView === 'review')"
            color="neutral"
            variant="outline"
            icon="i-lucide-rotate-cw"
            @click="onRetake"
          >
            Пройти ещё раз
          </UButton>
          <UButton color="primary" :icon="finishIcon" @click="emitFinish">Закрыть</UButton>
        </div>

        <!-- Пейджер вопроса -->
        <div v-else-if="page > 0" class="shrink-0">
          <!-- Навигация над полосой (тусклее, только для перехода) -->
          <div class="px-5 pb-2 flex items-center justify-between gap-3 min-h-9">
            <UButton v-if="canGoBack" color="neutral" variant="ghost" size="md" class="text-dimmed hover:text-default" leading-icon="i-lucide-arrow-left" @click="prev">Назад</UButton>
            <span v-else />
            <UButton v-if="!timeUp && reviewActive && nextUnansweredPage" color="neutral" variant="ghost" size="md" class="text-dimmed hover:text-default" :disabled="revealing" trailing-icon="i-lucide-arrow-right" @click="page = nextUnansweredPage">Далее</UButton>
            <UButton v-else-if="!timeUp && !isLastPage && !reviewActive && !currentLocked" color="neutral" variant="ghost" size="md" class="text-dimmed hover:text-default" :disabled="revealing" trailing-icon="i-lucide-arrow-right" @click="advance">Далее</UButton>
            <span v-else />
          </div>
          <!-- Полоса + счётчик + основная кнопка -->
          <div class="border-t border-default px-5 py-3 flex items-center justify-between gap-3">
            <span class="text-xs text-dimmed tabular-nums">{{ page }} / {{ view.length }}</span>
            <UButton v-if="timeUp" color="primary" size="md" :icon="finishIcon" :loading="submitting" @click="finalize">Завершить</UButton>
            <UButton v-else-if="isLastPage || reviewActive" color="primary" size="md" :icon="finishIcon" :loading="submitting" :disabled="revealing" @click="onFinishClick">{{ finishLabel }}</UButton>
            <UButton v-else-if="currentLocked" color="primary" size="md" trailing-icon="i-lucide-arrow-right" :disabled="revealing" @click="advance">Дальше</UButton>
            <UButton v-else color="primary" size="md" icon="i-lucide-check" :disabled="!currentQuestion || !isAnswered(currentQuestion) || revealing" @click="onAnswer">Ответить</UButton>
          </div>
        </div>
      </div>
    </div>

    <!-- Подтверждение завершения -->
    <UModal v-model:open="finishOpen" :title="`${finishLabel}?`" description="">
      <template #body>
        <p class="text-default">{{ isPoll ? 'Вы уверены, что хотите отдать голос? После отправки изменить его будет нельзя.' : 'Вы уверены, что хотите завершить и отправить ответы? После отправки изменить их будет нельзя.' }}</p>
      </template>
      <template #footer>
        <div class="flex justify-end gap-3 w-full">
          <UButton color="neutral" variant="outline" @click="finishOpen = false">Отмена</UButton>
          <UButton color="primary" :icon="finishIcon" :loading="submitting" @click="finalize">{{ finishLabel }}</UButton>
        </div>
      </template>
    </UModal>

    <!-- Неотвеченные -->
    <UModal v-model:open="unansweredOpen" title="Остались неотвеченные вопросы" description="">
      <template #body>
        <p class="text-default">
          Вы не ответили на {{ unanswered.length }} {{ plural(unanswered.length, 'вопрос', 'вопроса', 'вопросов') }}.
          <template v-if="hasRequiredUnanswered"> Среди них есть обязательные — без ответа на них завершить нельзя.</template>
          <template v-else> Можно вернуться к ним или завершить как есть.</template>
        </p>
      </template>
      <template #footer>
        <div class="flex justify-end gap-3 w-full">
          <UButton color="primary" icon="i-lucide-list-checks" @click="goToUnanswered">{{ unansweredLabel }}</UButton>
          <UButton v-if="!hasRequiredUnanswered" color="neutral" variant="outline" :icon="finishIcon" @click="finalize">{{ finishLabel }}</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>
