<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import {
  NOTIFICATION_KIND_ICON,
  NOTIFICATION_KIND_LABEL,
  notificationTitle,
  useNotifications,
  type PortalNotification,
} from '../composables/useNotifications';
import { formatWallDate } from '../composables/useProfileWall';
import { userAvatarSrc } from '../utils/userName';
import { useAppToast } from '../composables/useAppToast';

/** Колокольчик в шапке: поздравления, записи на вашей стене, ответы на комментарии. */
const router = useRouter();
const { error: toastError } = useAppToast();
const { items, unread, loading, loaded, error, refresh, markRead, markAllRead } = useNotifications();

const open = ref(false);

const unreadLabel = computed(() => (unread.value > 9 ? '9+' : String(unread.value)));
const ariaLabel = computed(() =>
  unread.value ? `Уведомления, непрочитанных: ${unread.value}` : 'Уведомления',
);

watch(open, (v) => {
  if (v) void refresh(true);
});

function onVisibility() {
  if (document.visibilityState === 'visible') void refresh();
}

let removeAfterEach: (() => void) | null = null;

onMounted(() => {
  void refresh(true);
  removeAfterEach = router.afterEach(() => void refresh());
  document.addEventListener('visibilitychange', onVisibility);
});

onUnmounted(() => {
  removeAfterEach?.();
  document.removeEventListener('visibilitychange', onVisibility);
});

function openNotification(n: PortalNotification) {
  void markRead([n.id]);
  open.value = false;
  if (n.kind === 'event_reminder' && n.reminder) {
    if (n.reminder.eventId) void router.push(`/events/${n.reminder.eventId}`);
    else void router.push({ path: '/calendar', query: { date: n.reminder.date } });
    return;
  }
  if (n.kind === 'devblog' && n.newsId) {
    void router.push(`/news/${n.newsId}`);
    return;
  }
  if (n.kind === 'comment_reply' && n.newsId && n.commentId) {
    void router.push({ path: `/news/${n.newsId}`, query: { comment: String(n.commentId) } });
    return;
  }
  // Остальные виды — записи на вашей стене.
  void router.push({ name: 'profile' });
}

/** «Завтра, 09:00 · Переговорная» */
function reminderLine(n: PortalNotification): string {
  const r = n.reminder;
  if (!r) return '';
  const d = new Date(`${r.date}T00:00:00`);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const days = Math.round((d.getTime() - today.getTime()) / 86_400_000);
  const day =
    days === 1
      ? 'Завтра'
      : days === 0
        ? 'Сегодня'
        : d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
  const time = r.timeStart && r.timeEnd ? `${r.timeStart}–${r.timeEnd}` : r.timeStart;
  return [time ? `${day}, ${time}` : day, r.location].filter(Boolean).join(' · ');
}

async function readAll() {
  if (!(await markAllRead())) toastError('Не удалось отметить уведомления', 'Попробуйте ещё раз.');
}
</script>

<template>
  <UPopover v-model:open="open" :content="{ align: 'end', side: 'bottom', sideOffset: 8 }">
    <UTooltip text="Уведомления" :content="{ side: 'bottom' }">
      <UChip :show="unread > 0" :text="unreadLabel" size="3xl" color="primary" inset>
        <UButton
          color="neutral"
          variant="outline"
          size="md"
          square
          class="h-8 shrink-0"
          icon="i-lucide-bell"
          :aria-label="ariaLabel"
        />
      </UChip>
    </UTooltip>

    <template #content>
      <div class="w-[min(24rem,calc(100vw-2rem))] flex flex-col">
        <div class="flex items-center justify-between gap-2 px-4 py-3 border-b border-default">
          <p class="text-sm font-semibold text-highlighted">Уведомления</p>
          <UButton
            v-if="unread > 0"
            label="Прочитать все"
            color="neutral"
            variant="link"
            size="xs"
            @click="readAll"
          />
        </div>

        <div class="max-h-[min(28rem,70vh)] overflow-y-auto p-1">
          <div v-if="loading && !loaded" class="flex flex-col gap-1 p-2">
            <div v-for="n in 3" :key="n" class="flex items-start gap-3 py-2">
              <USkeleton class="size-8 rounded-full shrink-0" />
              <div class="flex-1 flex flex-col gap-1.5">
                <USkeleton class="h-3 w-1/2" />
                <USkeleton class="h-3 w-5/6" />
              </div>
            </div>
          </div>

          <div v-else-if="error && !items.length" class="flex flex-col items-center gap-2 px-4 py-6 text-center">
            <p class="text-sm text-error">{{ error }}</p>
            <UButton label="Повторить" color="neutral" variant="outline" size="xs" @click="refresh(true)" />
          </div>

          <UEmpty
            v-else-if="!items.length"
            variant="naked"
            icon="i-lucide-bell-off"
            title="Уведомлений пока нет"
            description="Здесь появятся поздравления, записи коллег на вашей стене, ответы на ваши комментарии и напоминания о завтрашних событиях."
            class="py-6"
          />

          <ul v-else class="flex flex-col">
            <li v-for="n in items" :key="n.id">
              <button
                type="button"
                class="w-full flex items-start gap-3 rounded-md px-3 py-2.5 text-left transition-colors hover:bg-elevated focus-visible:outline-2 focus-visible:outline-primary"
                @click="openNotification(n)"
              >
                <UAvatar v-if="n.actor" :src="userAvatarSrc(n.actor)" :alt="n.actor.name" size="md" class="shrink-0" />
                <span
                  v-else
                  class="grid size-8 shrink-0 place-items-center rounded-full bg-elevated"
                  :style="n.reminder?.color ? { backgroundColor: `${n.reminder.color}33` } : undefined"
                >
                  <UIcon :name="NOTIFICATION_KIND_ICON[n.kind]" class="size-4 text-highlighted" aria-hidden="true" />
                </span>
                <span class="min-w-0 flex-1 flex flex-col gap-0.5">
                  <span class="flex items-center gap-1.5 text-xs text-muted">
                    <UIcon :name="NOTIFICATION_KIND_ICON[n.kind]" class="size-3.5 shrink-0" aria-hidden="true" />
                    {{ NOTIFICATION_KIND_LABEL[n.kind] }}
                  </span>
                  <span class="text-sm text-highlighted" :class="n.read ? 'font-medium' : 'font-semibold'">
                    {{ notificationTitle(n) }}
                  </span>
                  <span v-if="n.reminder" class="text-sm text-toned">{{ reminderLine(n) }}</span>
                  <span v-else-if="n.excerpt" class="text-sm text-toned line-clamp-2">{{ n.excerpt }}</span>
                  <span class="text-xs text-dimmed">{{ formatWallDate(n.createdAt) }}</span>
                </span>
                <span v-if="!n.read" class="mt-1.5 size-2 shrink-0 rounded-full bg-primary">
                  <span class="sr-only">Не прочитано</span>
                </span>
              </button>
            </li>
          </ul>
        </div>
      </div>
    </template>
  </UPopover>
</template>
