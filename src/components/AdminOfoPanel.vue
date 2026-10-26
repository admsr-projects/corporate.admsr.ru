<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useOfoTree, type OfoPosition, type OfoUnit } from '../composables/useOfoTree';
import { useUsersData, type AdminUserRow } from '../composables/useUsersData';
import { userAvatarSrc } from '../utils/userName';
import { plural } from '../pages/Courses/courseDuration';
import { useAppToast } from '../composables/useAppToast';

const emit = defineEmits<{ (e: 'open-user', user: AdminUserRow): void }>();

const {
  categories,
  units,
  loading,
  error,
  ensureLoaded,
  reload,
  rootUnitsOf,
  childrenOf,
  hasChildren,
  unitById,
  pathLabel,
  fetchPositions,
  addPosition,
  addUnit,
} = useOfoTree();
ensureLoaded();

const { success } = useAppToast();

const { users, loading: usersLoading, ensureLoaded: ensureUsersLoaded } = useUsersData();

const query = reactive({ q: '' });
const expanded = reactive<Set<number>>(new Set());

const sortedCategories = computed(() =>
  [...categories.value].sort((a, b) => a.sort_order - b.sort_order || a.id - b.id),
);

const totals = computed(() => ({
  categories: categories.value.length,
  units: units.value.length,
}));

const isSearching = computed(() => query.q.trim().length > 0);
const matches = computed(() => {
  const q = query.q.trim().toLowerCase();
  if (!q) return [];
  return units.value
    .filter((u) => u.name.toLowerCase().includes(q))
    .sort((a, b) => a.name.localeCompare(b.name, 'ru'));
});
const categoryName = (id: number) => categories.value.find((c) => c.id === id)?.name ?? '';
const parentLabel = (u: OfoUnit) => (u.parent_id != null ? pathLabel(u.parent_id) : categoryName(u.category_id));

function flatten(categoryId: number): { unit: OfoUnit; depth: number }[] {
  const out: { unit: OfoUnit; depth: number }[] = [];
  const walk = (unit: OfoUnit, depth: number) => {
    out.push({ unit, depth });
    for (const ch of childrenOf(unit.id)) walk(ch, depth + 1);
  };
  for (const root of rootUnitsOf(categoryId)) walk(root, 0);
  return out;
}
/** Сумма участников по всему поддереву (узел + все дочерние). */
function subtreeUserCount(unit: OfoUnit): number {
  let sum = unit.user_count ?? 0;
  for (const ch of childrenOf(unit.id)) sum += subtreeUserCount(ch);
  return sum;
}

function isVisible(unit: OfoUnit): boolean {
  let parentId = unit.parent_id;
  while (parentId != null) {
    if (!expanded.has(parentId)) return false;
    parentId = unitById.value.get(parentId)?.parent_id ?? null;
  }
  return true;
}
function toggle(id: number) {
  if (expanded.has(id)) expanded.delete(id);
  else expanded.add(id);
}
function expandAll() {
  for (const u of units.value) if (hasChildren(u.id)) expanded.add(u.id);
}
function collapseAll() {
  expanded.clear();
}

// ── Карточка подразделения ────────────────────────────────────────────────────
const detailOpen = ref(false);
const selectedId = ref<number | null>(null);
const withNested = ref(false);
const positions = ref<OfoPosition[]>([]);
const positionsLoading = ref(false);
let positionsSeq = 0;

const selected = computed(() => (selectedId.value != null ? unitById.value.get(selectedId.value) ?? null : null));
const selectedParentPath = computed(() => (selected.value ? parentLabel(selected.value) : ''));
const selectedChildren = computed(() => (selected.value ? childrenOf(selected.value.id) : []));
const selectedParent = computed(() =>
  selected.value?.parent_id != null ? unitById.value.get(selected.value.parent_id) ?? null : null,
);

function subtreeIds(unitId: number): Set<number> {
  const ids = new Set<number>([unitId]);
  const walk = (id: number) => {
    for (const ch of childrenOf(id)) {
      ids.add(ch.id);
      walk(ch.id);
    }
  };
  walk(unitId);
  return ids;
}

const members = computed(() => {
  const u = selected.value;
  if (!u) return [];
  const ids = withNested.value ? subtreeIds(u.id) : new Set([u.id]);
  return users.value
    .filter((user) => ids.has(Number(user.ofo)))
    .sort((a, b) => a.fullName.localeCompare(b.fullName, 'ru'));
});

async function loadPositions(unit: OfoUnit) {
  const seq = ++positionsSeq;
  positions.value = [];
  positionsLoading.value = true;
  try {
    const list = await fetchPositions(unit.unit_number);
    if (seq === positionsSeq) positions.value = list;
  } catch {
    if (seq === positionsSeq) positions.value = [];
  } finally {
    if (seq === positionsSeq) positionsLoading.value = false;
  }
}

function openUnit(unit: OfoUnit) {
  selectedId.value = unit.id;
  withNested.value = false;
  detailOpen.value = true;
  ensureUsersLoaded();
  void loadPositions(unit);
}

/** Карточка сотрудника — отдельная панель; эту закрываем, чтобы не спорить слоями. */
function openUser(user: AdminUserRow) {
  detailOpen.value = false;
  emit('open-user', user);
}

// ── Добавление должности ──────────────────────────────────────────────────────
const addOpen = ref(false);
const addName = ref('');
const addSaving = ref(false);
const addError = ref('');

function openAddPosition() {
  addName.value = '';
  addError.value = '';
  addOpen.value = true;
}

function closeAddPosition() {
  addOpen.value = false;
}

async function submitAddPosition() {
  const unit = selected.value;
  const name = addName.value.trim();
  if (!unit || addSaving.value) return;
  if (!name) {
    addError.value = 'Введите название должности';
    return;
  }
  addSaving.value = true;
  addError.value = '';
  try {
    await addPosition(unit.unit_number, name);
    addOpen.value = false;
    success('Должность добавлена', `«${name}» — ${unit.name}`);
    void loadPositions(unit);
    void reload(); // обновить счётчик должностей в дереве
  } catch (e) {
    addError.value = e instanceof Error ? e.message : 'Не удалось добавить должность';
  } finally {
    addSaving.value = false;
  }
}

// ── Добавление подразделения ──────────────────────────────────────────────────
const unitAddOpen = ref(false);
const unitName = ref('');
const unitCategoryId = ref<number | null>(null);
/** Родитель нового подразделения; null — корневое в выбранной категории. */
const unitParent = ref<OfoUnit | null>(null);
const unitSaving = ref(false);
const unitError = ref('');

const categoryItems = computed(() => sortedCategories.value.map((c) => ({ label: c.name, value: c.id })));

function openAddUnit(parent: OfoUnit | null) {
  unitParent.value = parent;
  unitCategoryId.value = parent?.category_id ?? sortedCategories.value[0]?.id ?? null;
  unitName.value = '';
  unitError.value = '';
  unitAddOpen.value = true;
}

async function submitAddUnit() {
  const name = unitName.value.trim();
  if (unitSaving.value) return;
  if (!name) {
    unitError.value = 'Введите название подразделения';
    return;
  }
  if (unitCategoryId.value == null) {
    unitError.value = 'Выберите категорию';
    return;
  }
  unitSaving.value = true;
  unitError.value = '';
  try {
    const created = await addUnit(name, unitCategoryId.value, unitParent.value?.id ?? null);
    unitAddOpen.value = false;
    success('Подразделение добавлено', `«${created.name}»`);
    if (created.parent_id != null) expanded.add(created.parent_id);
    await reload();
  } catch (e) {
    unitError.value = e instanceof Error ? e.message : 'Не удалось добавить подразделение';
  } finally {
    unitSaving.value = false;
  }
}

// Переход во вложенное подразделение внутри той же панели.
watch(selectedId, (id, prev) => {
  if (id == null || prev == null || id === prev) return;
  const u = unitById.value.get(id);
  if (u) void loadPositions(u);
});
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-col sm:flex-row gap-3 sm:items-center">
      <UInput
        v-model="query.q"
        icon="i-lucide-search"
        size="lg"
        color="neutral"
        variant="outline"
        placeholder="Поиск по названию…"
        class="w-full sm:flex-1"
      />
      <div class="flex items-center gap-2">
        <UButton color="neutral" variant="outline" icon="i-lucide-unfold-vertical" @click="expandAll">Развернуть</UButton>
        <UButton color="neutral" variant="outline" icon="i-lucide-fold-vertical" @click="collapseAll">Свернуть</UButton>
        <UButton color="primary" icon="i-lucide-plus" :disabled="!categories.length" @click="openAddUnit(null)">Добавить ОФО</UButton>
      </div>
    </div>

    <p class="text-sm text-muted">
      Категорий: {{ totals.categories }} · подразделений: {{ totals.units }}. Нажмите на подразделение, чтобы увидеть сотрудников.
    </p>

    <div
      v-if="loading && !units.length"
      class="rounded-panel border border-default p-2 flex flex-col gap-1"
      aria-busy="true"
      aria-label="Загрузка структуры ОФО"
    >
      <div v-for="n in 6" :key="n" class="flex items-center justify-between gap-3 px-2 py-2">
        <USkeleton class="h-4 rounded" :class="n % 2 ? 'w-64' : 'w-48'" />
        <USkeleton class="h-5 w-16 rounded-full" />
      </div>
    </div>

    <UAlert
      v-else-if="error"
      color="warning"
      variant="subtle"
      icon="i-lucide-server"
      title="Не удалось загрузить структуру ОФО"
      :description="error"
    >
      <template #actions>
        <UButton color="warning" icon="i-lucide-rotate-ccw" @click="reload">Повторить</UButton>
      </template>
    </UAlert>

    <div v-else class="flex flex-col gap-0.5 rounded-panel border border-default p-2 max-h-[60vh] overflow-y-auto scrollbar-hide">
      <!-- Поиск -->
      <template v-if="isSearching">
        <UEmpty
          v-if="!matches.length"
          variant="naked"
          icon="i-lucide-search-x"
          title="Ничего не найдено"
          description="Проверьте название или очистите поиск."
          class="py-8"
        />
        <button
          v-for="u in matches"
          :key="`m-${u.id}`"
          type="button"
          class="flex items-center justify-between gap-3 rounded-lg px-2 py-1.5 text-left hover:bg-elevated focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          @click="openUnit(u)"
        >
          <div class="min-w-0">
            <div class="text-sm text-default truncate">{{ u.name }}</div>
            <div class="text-xs text-dimmed truncate">{{ parentLabel(u) }}</div>
          </div>
          <div class="flex items-center gap-1.5 shrink-0">
            <UBadge v-if="hasChildren(u.id)" color="primary" variant="subtle" size="sm">{{ subtreeUserCount(u) }} всего</UBadge>
            <UBadge color="neutral" variant="subtle" size="sm">{{ u.user_count ?? 0 }} сотр.</UBadge>
          </div>
        </button>
      </template>

      <!-- Дерево -->
      <template v-else>
        <template v-for="cat in sortedCategories" :key="`c-${cat.id}`">
          <div class="px-2 pt-2 pb-1 text-xs font-medium uppercase tracking-wide text-dimmed select-none">
            {{ cat.name }}
          </div>
          <template v-for="row in flatten(cat.id)" :key="row.unit.id">
            <div
              v-if="isVisible(row.unit)"
              class="flex items-center gap-1 rounded-lg hover:bg-elevated"
              :style="{ paddingLeft: `${row.depth * 18}px` }"
            >
              <UTooltip v-if="hasChildren(row.unit.id)" :text="expanded.has(row.unit.id) ? 'Свернуть' : 'Развернуть'">
                <UButton
                  color="neutral"
                  variant="ghost"
                  size="xs"
                  square
                  :icon="expanded.has(row.unit.id) ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
                  :aria-label="`${expanded.has(row.unit.id) ? 'Свернуть' : 'Развернуть'}: ${row.unit.name}`"
                  :aria-expanded="expanded.has(row.unit.id)"
                  @click="toggle(row.unit.id)"
                />
              </UTooltip>
              <span v-else class="inline-block size-6 shrink-0" aria-hidden="true" />
              <button
                type="button"
                class="flex flex-1 min-w-0 items-center justify-between gap-3 rounded-md px-1.5 py-1.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                @click="openUnit(row.unit)"
              >
                <span class="text-sm text-default truncate">{{ row.unit.name }}</span>
                <span class="flex items-center gap-1.5 shrink-0">
                  <UBadge v-if="hasChildren(row.unit.id)" color="primary" variant="subtle" size="sm">{{ subtreeUserCount(row.unit) }} всего</UBadge>
                  <UBadge color="neutral" variant="subtle" size="sm">{{ row.unit.user_count ?? 0 }} сотр.</UBadge>
                </span>
              </button>
            </div>
          </template>
        </template>
      </template>
    </div>

    <USlideover
      v-model:open="detailOpen"
      side="right"
      :title="selected?.name || 'Подразделение'"
      :description="selectedParentPath || undefined"
    >
      <template #body>
        <div v-if="selected" class="flex flex-col gap-6">
          <UButton
            v-if="selectedParent"
            color="neutral"
            variant="link"
            icon="i-lucide-arrow-left"
            class="self-start -mb-3 px-0"
            @click="selectedId = selectedParent.id"
          >
            {{ selectedParent.name }}
          </UButton>

          <div class="grid gap-3" :class="selectedChildren.length ? 'grid-cols-3' : 'grid-cols-2'">
            <div class="rounded-panel bg-elevated p-3">
              <div class="text-2xl font-semibold text-highlighted tabular-nums">{{ selected.user_count ?? 0 }}</div>
              <div class="text-xs text-muted">{{ plural(selected.user_count ?? 0, ['сотрудник', 'сотрудника', 'сотрудников']) }}</div>
            </div>
            <div v-if="selectedChildren.length" class="rounded-panel bg-elevated p-3">
              <div class="text-2xl font-semibold text-highlighted tabular-nums">{{ subtreeUserCount(selected) }}</div>
              <div class="text-xs text-muted">с вложенными</div>
            </div>
            <div class="rounded-panel bg-elevated p-3">
              <div class="text-2xl font-semibold text-highlighted tabular-nums">{{ selected.position_count ?? positions.length }}</div>
              <div class="text-xs text-muted">{{ plural(selected.position_count ?? positions.length, ['должность', 'должности', 'должностей']) }}</div>
            </div>
          </div>

          <section class="flex flex-col gap-3" aria-labelledby="ofo-members-title">
            <div class="flex items-center justify-between gap-3">
              <h3 id="ofo-members-title" class="text-base font-semibold text-highlighted">Сотрудники</h3>
              <USwitch v-if="selectedChildren.length" v-model="withNested" label="С вложенными" size="sm" />
            </div>

            <div v-if="usersLoading && !users.length" class="flex flex-col gap-2" aria-busy="true" aria-label="Загрузка сотрудников">
              <div v-for="n in 3" :key="n" class="flex items-center gap-3 p-2">
                <USkeleton class="size-9 rounded-full" />
                <div class="flex-1 flex flex-col gap-1.5">
                  <USkeleton class="h-3.5 w-1/2 rounded" />
                  <USkeleton class="h-3 w-1/3 rounded" />
                </div>
              </div>
            </div>

            <UEmpty
              v-else-if="!members.length"
              variant="naked"
              icon="i-lucide-user-x"
              title="Сотрудников нет"
              :description="selectedChildren.length && !withNested
                ? 'В самом подразделении никого нет — включите «С вложенными».'
                : 'Сотрудник попадает сюда, когда в профиле выбрано это ОФО.'"
              class="py-6"
            />

            <ul v-else class="flex flex-col gap-1">
              <li v-for="m in members" :key="m.id">
                <button
                  type="button"
                  class="w-full flex items-center gap-3 rounded-lg p-2 text-left hover:bg-elevated focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  @click="openUser(m)"
                >
                  <UAvatar :src="userAvatarSrc(m)" :alt="m.fullName" size="md" class="shrink-0" />
                  <span class="flex-1 min-w-0">
                    <span class="block text-sm font-medium text-highlighted truncate">{{ m.fullName }}</span>
                    <span class="block text-xs text-muted truncate">
                      {{ m.role || 'Должность не указана' }}<template v-if="withNested && Number(m.ofo) !== selected.id"> · {{ unitById.get(Number(m.ofo))?.name }}</template>
                    </span>
                  </span>
                  <UBadge v-if="m.status !== 'Активен'" color="error" variant="subtle" size="sm" class="shrink-0">Заблокирован</UBadge>
                  <UIcon name="i-lucide-chevron-right" class="size-4 text-dimmed shrink-0" aria-hidden="true" />
                </button>
              </li>
            </ul>
          </section>

          <section class="flex flex-col gap-3" aria-labelledby="ofo-children-title">
            <div class="flex items-center justify-between gap-3">
              <h3 id="ofo-children-title" class="text-base font-semibold text-highlighted">Вложенные подразделения</h3>
              <UButton color="neutral" variant="outline" size="sm" icon="i-lucide-plus" @click="openAddUnit(selected)">
                Добавить
              </UButton>
            </div>
            <p v-if="!selectedChildren.length" class="text-sm text-muted">Вложенных подразделений нет.</p>
            <ul v-else class="flex flex-col gap-1">
              <li v-for="ch in selectedChildren" :key="ch.id">
                <button
                  type="button"
                  class="w-full flex items-center justify-between gap-3 rounded-lg p-2 text-left hover:bg-elevated focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  @click="selectedId = ch.id"
                >
                  <span class="text-sm text-default truncate">{{ ch.name }}</span>
                  <span class="flex items-center gap-1.5 shrink-0">
                    <UBadge color="neutral" variant="subtle" size="sm">{{ subtreeUserCount(ch) }} сотр.</UBadge>
                    <UIcon name="i-lucide-chevron-right" class="size-4 text-dimmed" aria-hidden="true" />
                  </span>
                </button>
              </li>
            </ul>
          </section>

          <section class="flex flex-col gap-3" aria-labelledby="ofo-positions-title">
            <div class="flex items-center justify-between gap-3">
              <h3 id="ofo-positions-title" class="text-base font-semibold text-highlighted">Должности</h3>
              <UButton color="neutral" variant="outline" size="sm" icon="i-lucide-plus" @click="openAddPosition">
                Добавить
              </UButton>
            </div>
            <div v-if="positionsLoading" class="flex flex-wrap gap-2" aria-busy="true" aria-label="Загрузка должностей">
              <USkeleton v-for="n in 3" :key="n" class="h-6 w-32 rounded-md" />
            </div>
            <p v-else-if="!positions.length" class="text-sm text-muted">
              В справочнике для этого подразделения должностей нет.
            </p>
            <div v-else class="flex flex-wrap gap-2">
              <UBadge
                v-for="p in positions"
                :key="p.id"
                :color="p.is_head ? 'primary' : 'neutral'"
                variant="subtle"
                :icon="p.is_head ? 'i-lucide-crown' : undefined"
              >
                {{ p.name }}
              </UBadge>
            </div>
          </section>
        </div>
      </template>
    </USlideover>

    <UModal
      v-model:open="unitAddOpen"
      title="Новое подразделение"
      :description="unitParent ? `Будет вложено в «${unitParent.name}».` : 'Будет добавлено на верхний уровень выбранной категории.'"
    >
      <template #body>
        <UForm :state="{ name: unitName }" class="flex flex-col gap-4" @submit.prevent="submitAddUnit">
          <UFormField v-if="!unitParent" label="Категория" name="category">
            <USelect v-model="unitCategoryId" :items="categoryItems" size="xl" color="neutral" class="w-full" />
          </UFormField>
          <UFormField label="Название" name="name" :error="unitError || undefined">
            <UInput
              v-model="unitName"
              size="xl"
              color="neutral"
              placeholder="Например, Отдел информатизации"
              maxlength="300"
              autofocus
              class="w-full"
              @update:model-value="unitError = ''"
            />
          </UFormField>
        </UForm>
      </template>
      <template #footer>
        <div class="flex justify-end gap-3 w-full">
          <UButton color="neutral" variant="outline" size="xl" @click="unitAddOpen = false">Отмена</UButton>
          <UButton color="primary" size="xl" :loading="unitSaving" @click="submitAddUnit">Добавить</UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="addOpen"
      title="Новая должность"
      :description="selected ? `Будет доступна в подразделении «${selected.name}».` : undefined"
    >
      <template #body>
        <UForm :state="{ name: addName }" class="flex flex-col gap-2" @submit.prevent="submitAddPosition">
          <UFormField label="Название должности" name="name" :error="addError || undefined">
            <UInput
              v-model="addName"
              size="xl"
              color="neutral"
              placeholder="Например, Ведущий специалист"
              maxlength="200"
              autofocus
              class="w-full"
              @update:model-value="addError = ''"
            />
          </UFormField>
        </UForm>
      </template>
      <template #footer>
        <div class="flex justify-end gap-3 w-full">
          <UButton color="neutral" variant="outline" size="xl" @click="closeAddPosition">Отмена</UButton>
          <UButton color="primary" size="xl" :loading="addSaving" @click="submitAddPosition">Добавить</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>
