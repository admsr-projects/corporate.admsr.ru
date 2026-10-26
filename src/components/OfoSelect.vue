<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useOfoTree, type OfoUnit } from '../composables/useOfoTree';

const props = withDefaults(
  defineProps<{
    modelValue: number | null;
    disabled?: boolean;
    placeholder?: string;
    size?: 'md' | 'lg' | 'xl';
    /** Пункт «сбросить» вверху списка (для фильтров): выбирает null. */
    resetLabel?: string;
  }>(),
  { placeholder: 'Выберите подразделение', size: 'xl' },
);
const emit = defineEmits<{ (e: 'update:modelValue', v: number | null): void }>();

const { categories, units, loading, error, ensureLoaded, rootUnitsOf, childrenOf, hasChildren, unitById, pathLabel } = useOfoTree();
ensureLoaded();

const open = ref(false);
const expanded = reactive<Set<number>>(new Set());
const query = ref('');
const isSearching = computed(() => query.value.trim().length > 0);

const SEL = 'bg-primary/10 text-primary ring-1 ring-inset ring-primary/50';
const HOVER = 'hover:bg-elevated text-default';

const selectedLabel = computed(() => (props.modelValue != null ? pathLabel(props.modelValue) : ''));

const sortedCategories = computed(() =>
  [...categories.value].sort((a, b) => a.sort_order - b.sort_order || a.id - b.id),
);

const matches = computed(() => {
  const q = query.value.trim().toLowerCase();
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
function select(id: number | null) {
  emit('update:modelValue', id);
  open.value = false;
  query.value = '';
}

function expandPathTo(id: number | null) {
  if (id == null) return;
  let cur = unitById.value.get(id);
  while (cur?.parent_id != null) {
    expanded.add(cur.parent_id);
    cur = unitById.value.get(cur.parent_id);
  }
}
watch(() => props.modelValue, expandPathTo, { immediate: true });
watch(open, (v) => { if (v) expandPathTo(props.modelValue); });
</script>

<template>
  <UPopover v-model:open="open" :content="{ align: 'start', sideOffset: 6 }" :ui="{ content: 'w-(--reka-popper-anchor-width) p-0' }">
    <UButton
      color="neutral"
      variant="outline"
      :size="size"
      :disabled="disabled"
      trailing-icon="i-lucide-chevron-down"
      class="w-full justify-between"
    >
      <span class="truncate" :class="modelValue == null ? 'text-dimmed' : ''">
        {{ selectedLabel || placeholder }}
      </span>
    </UButton>

    <template #content>
      <div class="p-2">
        <div v-if="loading" class="text-sm text-muted px-1 py-2">Загрузка подразделений…</div>
        <div v-else-if="error" class="text-sm text-error px-1 py-2">{{ error }}</div>

        <div v-else class="flex flex-col gap-2">
          <UInput
            v-model="query"
            icon="i-lucide-search"
            placeholder="Поиск по названию…"
            size="md"
            color="neutral"
            autofocus
            class="w-full"
          />

          <div class="flex flex-col gap-1 max-h-72 overflow-y-auto scrollbar-hide px-0.5 py-0.5">
            <button
              v-if="resetLabel && !isSearching"
              type="button"
              class="w-full text-left rounded-lg px-2 py-1.5 text-sm transition-colors"
              :class="modelValue == null ? SEL : HOVER"
              @click="select(null)"
            >
              {{ resetLabel }}
            </button>
            <!-- Поиск -->
            <template v-if="isSearching">
              <p v-if="!matches.length" class="text-sm text-muted px-2 py-2">Ничего не найдено</p>
              <button
                v-for="u in matches"
                :key="`m-${u.id}`"
                type="button"
                class="flex flex-col items-start w-full text-left rounded-lg px-2 py-1.5 transition-colors"
                :class="modelValue === u.id ? SEL : HOVER"
                @click="select(u.id)"
              >
                <span class="text-sm truncate w-full">{{ u.name }}</span>
                <span class="text-xs text-dimmed truncate w-full">{{ parentLabel(u) }}</span>
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
                    class="flex items-center gap-1.5 w-full rounded-lg transition-colors"
                    :class="modelValue === row.unit.id ? SEL : HOVER"
                    :style="{ paddingLeft: `${row.depth * 18}px` }"
                  >
                    <button
                      v-if="hasChildren(row.unit.id)"
                      type="button"
                      class="shrink-0 p-1.5"
                      @click.stop="toggle(row.unit.id)"
                    >
                      <UIcon
                        :name="expanded.has(row.unit.id) ? 'i-lucide-chevron-down' : 'i-lucide-chevron-right'"
                        class="size-4 text-muted"
                      />
                    </button>
                    <span v-else class="inline-block size-4 shrink-0 ml-1.5" aria-hidden="true" />
                    <button
                      type="button"
                      class="flex-1 min-w-0 text-left py-1.5 pr-2"
                      @click="select(row.unit.id)"
                    >
                      <span class="text-sm truncate block">{{ row.unit.name }}</span>
                    </button>
                  </div>
                </template>
              </template>
            </template>
          </div>
        </div>
      </div>
    </template>
  </UPopover>
</template>
