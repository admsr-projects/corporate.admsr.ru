<script setup lang="ts">
/**
 * Поля «Начало», «Окончание», «Место», «Лимит мест» для создания и правки мероприятия
 * (ADR-054). Время — UInputTime в 24-часовом формате: «1300» набирается как 13:00.
 * Наружу — строки «HH:MM» (пусто — не указано) и число мест (null — без ограничения).
 */
import { computed } from 'vue';
import { Time } from '@internationalized/date';

withDefaults(defineProps<{ size?: 'md' | 'lg' | 'xl' }>(), { size: 'lg' });

const timeStart = defineModel<string>('timeStart', { default: '' });
const timeEnd = defineModel<string>('timeEnd', { default: '' });
const location = defineModel<string>('location', { default: '' });
const maxParticipants = defineModel<number | null>('maxParticipants', { default: null });

function toTime(s: string): Time | undefined {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s);
  return m ? new Time(Number(m[1]), Number(m[2])) : undefined;
}

function fromTime(t: { hour: number; minute: number } | null | undefined): string {
  return t ? `${String(t.hour).padStart(2, '0')}:${String(t.minute).padStart(2, '0')}` : '';
}

const startValue = computed({
  get: () => toTime(timeStart.value),
  set: (t) => {
    timeStart.value = fromTime(t);
  },
});

const endValue = computed({
  get: () => toTime(timeEnd.value),
  set: (t) => {
    timeEnd.value = fromTime(t);
  },
});

const endBeforeStart = computed(() => !!timeStart.value && !!timeEnd.value && timeEnd.value < timeStart.value);

// Пустое поле UInputNumber даёт null или NaN — оба значат «без ограничения».
const limitValue = computed({
  get: () => maxParticipants.value ?? undefined,
  set: (v) => {
    const n = Number(v);
    maxParticipants.value = Number.isFinite(n) && n > 0 ? Math.round(n) : null;
  },
});
</script>

<template>
  <div class="grid grid-cols-2 gap-3">
    <UFormField label="Начало" name="time_start" hint="Необязательно">
      <UInputTime v-model="startValue" :size="size" :hour-cycle="24" granularity="minute" class="w-full" />
    </UFormField>
    <UFormField
      label="Окончание"
      name="time_end"
      :error="endBeforeStart ? 'Раньше начала' : undefined"
    >
      <UInputTime
        v-model="endValue"
        :size="size"
        :hour-cycle="24"
        granularity="minute"
        :disabled="!timeStart"
        class="w-full"
      />
    </UFormField>
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

  <UFormField
    label="Лимит мест"
    name="max_participants"
    description="Сколько человек может записаться. Пусто — без ограничения."
  >
    <UInputNumber
      v-model="limitValue"
      :size="size"
      :min="1"
      :max="100000"
      :step="1"
      placeholder="Без ограничения"
      :format-options="{ maximumFractionDigits: 0 }"
      class="w-48"
    />
  </UFormField>
</template>
