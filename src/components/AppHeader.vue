<script setup>
import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useHeaderUser } from '../composables/useHeaderUser';
import { usePortalBreadcrumbs } from '../composables/usePortalNavigation';
import { currentRole, setRole } from '../stores/role';
import { clearAuthStorage } from '../composables/useAuthSession';
import NotificationsBell from './NotificationsBell.vue';
import {
  currentFontLabel,
  currentRadiusLabel,
  patchAppTheme,
  randomAppTheme,
  resetAppTheme,
  useAppConfig,
} from '../composables/useAppConfig';
import {
  FONT_OPTIONS,
  NEUTRAL_COLOR_LABELS,
  NEUTRAL_COLORS,
  PRIMARY_COLOR_LABELS,
  PRIMARY_COLORS,
  RADIUS_OPTIONS,
} from '../composables/useUiTheme';

const { headerName, avatarSrc, loading, canToggleAdminRole } = useHeaderUser();
const breadcrumbItems = usePortalBreadcrumbs();
const appConfig = useAppConfig();

/**
 * Длинный путь сворачиваем: первый элемент и два последних, середина — под «…».
 * Скрытые уровни доступны из меню, так что переход по ним не теряется.
 */
const visibleBreadcrumbs = computed(() => {
  const items = breadcrumbItems.value;
  if (items.length <= 3) return items;
  return [items[0], { slot: 'ellipsis' }, ...items.slice(-2)];
});

const hiddenBreadcrumbMenu = computed(() => {
  const items = breadcrumbItems.value;
  if (items.length <= 3) return [];
  return [items.slice(1, -2).map((i) => ({ label: i.label, icon: i.icon, to: i.to }))];
});

/** Подсказку у крошки показываем, только если подпись действительно обрезана. */
const truncatedCrumbs = ref({});

function markCrumbTruncation(index, event) {
  const el = event.currentTarget;
  truncatedCrumbs.value = { ...truncatedCrumbs.value, [index]: el.scrollWidth > el.clientWidth + 1 };
}

watch(breadcrumbItems, () => {
  truncatedCrumbs.value = {};
});

const props = defineProps({
  isDark: {
    type: Boolean,
    default: false,
  },
  colorMode: {
    type: String,
    default: 'light',
    validator: (value) => ['light', 'dark', 'system'].includes(value),
  },
  activeNav: {
    type: String,
    default: 'events',
  },
});

const emit = defineEmits(['toggle-theme']);

const router = useRouter();
const userMenuOpen = ref(false);
const isAdminRole = ref(currentRole.value === 'admin');

watch(isAdminRole, (v) => {
  if (canToggleAdminRole.value) setRole(v ? 'admin' : 'user');
});
watch(currentRole, (r) => {
  isAdminRole.value = r === 'admin';
});
watch(canToggleAdminRole, (can) => {
  if (!can) setRole('user');
}, { immediate: true });

async function logout() {
  try {
    const user = JSON.parse(localStorage.getItem('auth-user') || 'null');
    if (user?.id) {
      await fetch('/api/logout.php', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: user.id }),
      });
    }
  } finally {
    clearAuthStorage();
    router.push({ name: 'login' });
  }
}

const colorModeTabItems = [
  { value: 'light', icon: 'i-lucide-sun', label: 'Светлая' },
  { value: 'dark', icon: 'i-lucide-moon', label: 'Тёмная' },
  { value: 'system', icon: 'i-lucide-monitor', label: 'Система' },
];

const roleTabItems = [
  { value: 'user', label: 'Пользователь' },
  { value: 'admin', label: 'Администратор' },
];

const colorModeTab = computed({
  get: () => props.colorMode,
  set: (value) => emit('toggle-theme', undefined, value),
});

const roleTab = computed({
  get: () => (isAdminRole.value ? 'admin' : 'user'),
  set: (value) => {
    isAdminRole.value = value === 'admin';
  },
});

const colorModeIcon = computed(() => {
  if (props.colorMode === 'dark') return 'i-lucide-moon';
  if (props.colorMode === 'system') return 'i-lucide-monitor';
  return 'i-lucide-sun';
});

const profileTriggerLabel = computed(() => (
  headerName.value ? `Меню профиля: ${headerName.value}` : 'Меню профиля'
));

function keepMenuOpen(e) {
  e.preventDefault();
}

const userMenuItems = computed(() => {
  const groups = [
    [
      {
        label: headerName.value || 'Профиль',
        avatar: {
          src: avatarSrc.value,
          loading: 'lazy',
        },
        type: 'label',
      },
    ],
    [
      {
        label: 'Профиль',
        icon: 'i-lucide-user',
        to: '/profile',
      },
    ],
  ];

  if (canToggleAdminRole.value) {
    groups.push([
      {
        label: 'Роль',
        slot: 'role',
        class: 'cursor-default',
        onSelect: keepMenuOpen,
      },
      // Только в роли администратора — как и сама страница (requiresAdmin), ADR-052.
      ...(currentRole.value === 'admin'
        ? [
            { label: 'Дэшборд администратора', icon: 'i-lucide-layout-dashboard', to: '/admin' },
            { label: 'Редактировать девблог', icon: 'i-lucide-notebook-pen', to: '/admin/devblog' },
          ]
        : []),
    ]);
  }

  groups.push(
    [
      {
        label: 'Основной',
        slot: 'chip',
        chip: appConfig.ui.colors.primary,
        content: { align: 'end', collisionPadding: 16 },
        children: PRIMARY_COLORS.map((c) => ({
          label: PRIMARY_COLOR_LABELS[c],
          chip: c,
          slot: 'chip',
          type: 'checkbox',
          checked: appConfig.ui.colors.primary === c,
          onUpdateChecked(checked) {
            if (checked) patchAppTheme({ primary: c });
          },
          onSelect(e) {
            e.preventDefault();
          },
        })),
      },
      {
        label: 'Нейтральный',
        slot: 'chip',
        chip: appConfig.ui.colors.neutral,
        content: { align: 'end', collisionPadding: 16 },
        children: NEUTRAL_COLORS.map((c) => ({
          label: NEUTRAL_COLOR_LABELS[c],
          chip: c,
          slot: 'chip',
          type: 'checkbox',
          checked: appConfig.ui.colors.neutral === c,
          onUpdateChecked(checked) {
            if (checked) patchAppTheme({ neutral: c });
          },
          onSelect(e) {
            e.preventDefault();
          },
        })),
      },
      {
        label: 'Шрифт',
        icon: 'i-lucide-type',
        kbds: [currentFontLabel()],
        content: { align: 'end', collisionPadding: 16 },
        children: FONT_OPTIONS.map((font) => ({
          label: font.label,
          type: 'checkbox',
          checked: appConfig.ui.font === font.id,
          onUpdateChecked(checked) {
            if (checked) patchAppTheme({ font: font.id });
          },
          onSelect(e) {
            e.preventDefault();
          },
        })),
      },
      {
        label: 'Скругление',
        icon: 'i-lucide-radius',
        kbds: [currentRadiusLabel()],
        content: { align: 'end', collisionPadding: 16 },
        children: RADIUS_OPTIONS.map((radius) => ({
          label: radius.label,
          type: 'checkbox',
          checked: appConfig.ui.radius === radius.id,
          onUpdateChecked(checked) {
            if (checked) patchAppTheme({ radius: radius.id });
          },
          onSelect(e) {
            e.preventDefault();
          },
        })),
      },
    ],
    [
      {
        label: 'Случайная тема',
        icon: 'i-lucide-dices',
        onSelect(e) {
          e.preventDefault();
          randomAppTheme();
        },
      },
      {
        label: 'Сбросить',
        icon: 'i-lucide-rotate-ccw',
        onSelect(e) {
          e.preventDefault();
          resetAppTheme();
        },
      },
    ],
    [
      {
        label: 'Система',
        icon: colorModeIcon.value,
        slot: 'theme',
        class: 'cursor-default',
        onSelect: keepMenuOpen,
      },
      {
        label: 'Выйти',
        icon: 'i-lucide-log-out',
        color: 'error',
        onSelect() {
          void logout();
        },
      },
    ],
  );

  return groups;
});
</script>

<template>
  <!--
    `@container` на корне: ширина шапки зависит от сайдбара (его тянут в 220–400px
    и сворачивают), поэтому поиск подстраивается под ширину шапки, а не окна.
  -->
  <UDashboardNavbar
    :ui="{
      root: '@container h-[60px] shrink-0 border-0 mx-2 sm:mx-4 mt-4 rounded-panel bg-elevated px-2 sm:px-4',
      left: 'min-w-0 flex-1',
      right: 'gap-2 sm:gap-4 shrink-0 min-w-0',
      title: 'min-w-0',
    }"
  >
    <template #title>
      <UBreadcrumb
        :items="visibleBreadcrumbs"
        class="min-w-0"
        :ui="{
          root: 'min-w-0',
          list: 'min-w-0 flex-nowrap overflow-hidden',
          item: 'min-w-0',
          linkLeadingIcon: 'size-5 text-muted',
          linkLabel: 'text-sm font-medium truncate',
          separatorIcon: 'size-5 text-muted',
        }"
      >
        <template #item-label="{ item, index }">
          <UTooltip :text="item.label" :disabled="!truncatedCrumbs[index]" :content="{ side: 'bottom' }">
            <span class="block truncate" @mouseenter="markCrumbTruncation(index, $event)">{{ item.label }}</span>
          </UTooltip>
        </template>

        <template #ellipsis>
          <!-- span-обёртка: подсказка и меню не делят один триггер -->
          <UTooltip text="Показать весь путь" :content="{ side: 'bottom' }">
            <span class="inline-flex">
              <UDropdownMenu :items="hiddenBreadcrumbMenu" :content="{ align: 'start', sideOffset: 8 }">
                <UButton
                  type="button"
                  color="neutral"
                  variant="ghost"
                  size="xs"
                  square
                  icon="i-lucide-ellipsis"
                  aria-label="Показать весь путь"
                />
              </UDropdownMenu>
            </span>
          </UTooltip>
        </template>
      </UBreadcrumb>
    </template>

    <template #right>
      <div class="flex items-center gap-2 min-w-0">
        <!-- Узкая шапка: только иконка, подпись — в подсказке -->
        <UDashboardSearchButton
          collapsed
          :tooltip="{ content: { side: 'bottom' }, kbds: ['meta', 'K'] }"
          color="neutral"
          variant="outline"
          size="md"
          class="inline-flex h-8 shrink-0 @4xl:hidden"
        />
        <!-- Средняя: короткая подпись -->
        <UDashboardSearchButton
          label="Поиск…"
          color="neutral"
          variant="outline"
          size="md"
          class="hidden @4xl:inline-flex @6xl:hidden h-8 w-44 justify-start"
          :kbds="['meta', 'K']"
        />
        <!-- Широкая: полная подпись -->
        <UDashboardSearchButton
          label="Искать сотрудника, памятку, документ..."
          color="neutral"
          variant="outline"
          size="md"
          class="hidden @6xl:inline-flex h-8 min-w-0 w-[340px] justify-start"
          :kbds="['meta', 'K']"
          :ui="{
            label: 'truncate',
          }"
        />

        <NotificationsBell />

        <UDropdownMenu
          v-model:open="userMenuOpen"
          :items="userMenuItems"
          :disabled="loading"
          :content="{ align: 'end', side: 'bottom', sideOffset: 8 }"
          :ui="{ content: 'w-72' }"
        >
          <template #default="{ open }">
            <UButton
              type="button"
              color="neutral"
              variant="outline"
              size="md"
              class="h-8 min-w-0 shrink-0 data-[state=open]:bg-elevated"
              :aria-label="profileTriggerLabel"
              :aria-expanded="open"
              aria-haspopup="menu"
            >
              <template v-if="loading">
                <USkeleton class="size-5 rounded-full" />
                <USkeleton class="h-3.5 w-28 hidden sm:block" />
              </template>
              <template v-else>
                <UAvatar
                  :src="avatarSrc"
                  :alt="headerName"
                  size="2xs"
                  icon="i-lucide-user"
                />
                <span class="hidden sm:inline text-sm font-medium leading-none text-highlighted truncate max-w-[140px]">
                  {{ headerName }}
                </span>
                <UIcon
                  name="i-lucide-chevron-down"
                  class="size-5 text-muted shrink-0 transition-transform"
                  :class="open ? 'rotate-180' : ''"
                />
              </template>
            </UButton>
          </template>

          <template #chip-leading="{ item }">
            <span class="inline-flex items-center justify-center shrink-0 size-5">
              <span
                class="rounded-full size-2 ring ring-bg bg-(--chip-light) dark:bg-(--chip-dark)"
                :style="{
                  '--chip-light': `var(--color-${item.chip}-500)`,
                  '--chip-dark': `var(--color-${item.chip}-400)`,
                }"
              />
            </span>
          </template>

        <template #role>
          <UTabs
            v-model="roleTab"
            :items="roleTabItems"
            size="xs"
            color="neutral"
            variant="pill"
            :content="false"
            activation-mode="manual"
            class="w-full"
            :ui="{ list: 'w-full' }"
            @click.stop
          />
        </template>

        <template #theme-trailing>
          <UTabs
            v-model="colorModeTab"
            :items="colorModeTabItems"
            size="xs"
            color="neutral"
            variant="pill"
            :content="false"
            activation-mode="manual"
            class="w-24"
            :ui="{
              list: 'p-0.5 gap-0',
              trigger: 'p-1',
              leadingIcon: 'size-3.5',
              label: 'sr-only',
            }"
            @click.stop
          />
        </template>
      </UDropdownMenu>
      </div>
    </template>
  </UDashboardNavbar>
</template>
