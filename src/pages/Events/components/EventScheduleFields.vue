<script setup lang="ts">
/**
 * Времена и место мероприятия в создании и правке (ADR-054, ADR-056). У каждого времени —
 * начало, окончание и свой лимит мест; «+ Добавить время» — под последним временем.
 * Время — UInputTime в 24-часовом формате: «1300» набирается как 13:00. Удаление времени,
 * на которое уже записались, — через подтверждение: записи на него удалятся.
 */
import { ref } from 'vue';
import { Time } from '@internationalized/date';
import { emptySlotDraft, slotTimeLabel, type EventSlotDraft } from '../../../composables/eventSchedule';

withDefaults(defineProps<{ size?: 'md' | 'lg' | 'xl' }>(), { size: 'lg' });

const location = defineModel<string>('location', { default: '' });
const slots = defineModel<EventSlotDraft[]>('slots', { required: true });

function toTime(s: string): Time | undefined {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s);
  return m ? new Time(Number(m[1]), Number(m[2])) : undefined;
}

function fromTime(t: { hour: number; minute: number } | null | undefined): string {
  return t ? `${String(t.hour).padStart(2, '0')}:${String(t.minute).padStart(2, '0')}` : '';
}

function patch(i: number, part: Partial<EventSlotDraft>) {
  slots.value = slots.value.map((s, j) => {
    if (j !== i) return s;
    const next = { ...s, ...part };
    // Без начала окончания нет.
    if (!next.timeStart) next.timeEnd = '';
    return next;
  });
}

// Пустое поле UInputNumber даёт null или NaN — оба значат «без ограничения».
function setLimit(i: number, v: unknown) {
  const n = Number(v);
  patch(i, { maxParticipants: v != null && Number.isFinite(n) && n > 0 ? Math.round(n) : null });
}

function endBeforeStart(s: EventSlotDraft): boolean {
  return !!s.timeStart && !!s.timeEnd && s.timeEnd < s.timeStart;
}

/** Новое время начинается там, где закончилось последнее. */
function addSlot() {
  const last = slots.value[slots.value.length - 1];
  slots.value = [...slots.value, emptySlotDraft(last?.timeEnd || '')];
}

const confirmIndex = ref<number | null>(null);

function askRemove(i: number) {
  if (slots.value[i]?.participantsCount) confirmIndex.value = i;
  else removeSlot(i);
}

function closeConfirm() {
  confirmIndex.value = null;
}

function onConfirmOpen(open: boolean) {
  if (!open) closeConfirm();
}

function removeSlot(i: number) {
  slots.value = slots.value.filter((_, j) => j !== i);
  confirmIndex.value = null;
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <div
      v-for="(slot, i) in slots"
      :key="slot.id || `new-${i}`"
      class="flex flex-col gap-3 rounded-lg border border-default p-3"
    >
      <div v-if="slots.length > 1" class="flex items-center justify-between gap-2">
        <p class="text-sm font-medium text-highlighted">
          Время {{ i + 1 }}
          <span v-if="slot.participantsCount" class="font-normal text-muted">· записалось {{ slot.participantsCount }}</span>
        </p>
        <UTooltip text="Удалить время">
          <UButton
            color="neutral"
            variant="ghost"
            size="xs"
            icon="i-lucide-trash-2"
            :aria-label="`Удалить время ${i + 1}`"
            @click="askRemove(i)"
          />
        </UTooltip>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <UFormField label="Начало" hint="Необязательно">
          <UInputTime
            :model-value="toTime(slot.timeStart)"
            :size="size"
            :hour-cycle="24"
            granularity="minute"
            class="w-full"
            @update:model-value="(t: any) => patch(i, { timeStart: fromTime(t) })"
          />
        </UFormField>
        <UFormField label="Окончание" :error="endBeforeStart(slot) ? 'Раньше начала' : undefined">
          <UInputTime
            :model-value="toTime(slot.timeEnd)"
            :size="size"
            :hour-cycle="24"
            granularity="minute"
            :disabled="!slot.timeStart"
            class="w-full"
            @update:model-value="(t: any) => patch(i, { timeEnd: fromTime(t) })"
          />
        </UFormField>
      </div>

      <UFormField label="Лимит мест" description="Пусто — без ограничения.">
        <UInputNumber
          :model-value="slot.maxParticipants ?? undefined"
          :size="size"
          :min="1"
          :max="100000"
          :step="1"
          placeholder="Без ограничения"
          :format-options="{ maximumFractionDigits: 0 }"
          class="w-48"
          @update:model-value="(v: unknown) => setLimit(i, v)"
        />
      </UFormField>
    </div>

    <UButton
      color="neutral"
      variant="outline"
      icon="i-lucide-plus"
      :size="size === 'xl' ? 'lg' : 'md'"
      class="self-start"
      :disabled="slots.length >= 20"
      @click="addSlot"
    >
      Добавить время
    </UButton>
  </div>

  <UFormField label="Место" name="location" hint="Необязательно">
    <UInput
      v-model="location"
      :size="size"
      maxlength="300"
      placeholder="Например, Районный центр культуры, Белый Яр"
      class="w-full"
    />
  </UFormField>

  <UModal
    :open="confirmIndex !== null"
    title="Удалить время?"
    description="Записи сотрудников на это время тоже удалятся после сохранения."
    @update:open="onConfirmOpen"
  >
    <template #body>
      <p v-if="confirmIndex !== null" class="text-default">
        На время <strong>{{ slotTimeLabel(slots[confirmIndex]!) || 'без указания времени' }}</strong>
        записалось {{ slots[confirmIndex]!.participantsCount }}. Удалить его вместе с записями?
      </p>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="outline" @click="closeConfirm">Отмена</UButton>
        <UButton color="error" @click="removeSlot(confirmIndex!)">Удалить</UButton>
      </div>
    </template>
  </UModal>
</template>
