import { avatarUrlFromFilename, DEFAULT_PROFILE_AVATAR_FILENAME } from '../constants/profileAvatars';
import { clearAuthStorage } from './useAuthSession';

export type ProfileSnapshot = {
  id: number;
  firstname: string;
  surname: string;
  lastname: string;
  ofo: string | number;
  role: string;
  avatar_url: string;
  phone?: string;
  email?: string;
};

const SESSION_KEY_PREFIX = 'onboarding-complete:v1:';

export function onboardingSessionKey(userId: number): string {
  return `${SESSION_KEY_PREFIX}${userId}`;
}

export function isOfoUnset(ofo: unknown): boolean {
  const s = String(ofo ?? '').trim();
  return !s || s === '-1';
}

export function isProfileIncomplete(profile: Pick<ProfileSnapshot, 'ofo' | 'role' | 'avatar_url'>): boolean {
  if (isOfoUnset(profile.ofo)) return true;
  if (!String(profile.role ?? '').trim()) return true;
  if (!String(profile.avatar_url ?? '').trim()) return true;
  return false;
}

export function markOnboardingComplete(userId: number): void {
  if (typeof window === 'undefined') return;
  sessionStorage.setItem(onboardingSessionKey(userId), '1');
}

export function isOnboardingMarkedComplete(userId: number): boolean {
  if (typeof window === 'undefined') return false;
  return sessionStorage.getItem(onboardingSessionKey(userId)) === '1';
}

/**
 * Профиль для приветствия. `unauthorized` — сессия на сервере истекла (profile.php
 * отвечает 401 без входа): это повод выйти на страницу входа, а не показывать
 * приветствие. `error` — сеть или сервер недоступны.
 */
export type ProfileSnapshotResult =
  | { status: 'ok'; profile: ProfileSnapshot }
  | { status: 'unauthorized' }
  | { status: 'error' };

export async function loadProfileSnapshot(userId: number): Promise<ProfileSnapshotResult> {
  try {
    const res = await fetch(`/api/profile.php?id=${userId}`);
    if (res.status === 401) return { status: 'unauthorized' };
    const data = await res.json();
    if (!data.success || !data.data) return { status: 'error' };
    const p = data.data;
    return {
      status: 'ok',
      profile: {
        id: Number(p.id),
        firstname: p.firstname ?? '',
        surname: p.surname ?? '',
        lastname: p.lastname ?? '',
        ofo: p.ofo ?? '',
        role: p.role ?? '',
        avatar_url: p.avatar_url ?? '',
        phone: p.phone ?? '',
        email: p.email ?? '',
      },
    };
  } catch {
    return { status: 'error' };
  }
}

/**
 * `incomplete` — профиль не заполнен, нужно приветствие; `unauthorized` — сессия истекла,
 * нужен вход; `complete` / `error` — пропускаем дальше. Раньше любая неудача (в том числе
 * 401 после истечения сессии) считалась «профиль не заполнен», и вместо входа открывалось
 * приветствие с выбором подразделения — похожее на регистрацию.
 */
export type OnboardingCheck = 'complete' | 'incomplete' | 'unauthorized' | 'error';

export async function checkOnboarding(userId: number): Promise<OnboardingCheck> {
  const res = await loadProfileSnapshot(userId);
  if (res.status !== 'ok') return res.status;
  if (isProfileIncomplete(res.profile)) return 'incomplete';
  markOnboardingComplete(userId);
  return 'complete';
}

export async function userNeedsOnboarding(userId: number): Promise<boolean> {
  return (await checkOnboarding(userId)) === 'incomplete';
}

/** Сессия истекла: чистый выход и отметка для страницы входа («Сессия истекла. Войдите снова.»). */
export function expireSession(): void {
  clearAuthStorage();
  try {
    sessionStorage.setItem('session-expired', '1');
  } catch {
    /* хранилище недоступно */
  }
}

export type SaveOnboardingPayload = {
  id: number;
  ofo: string;
  role: string;
  avatar_url: string;
};

export async function saveOnboardingProfile(payload: SaveOnboardingPayload): Promise<boolean> {
  try {
    const res = await fetch('/api/profile.php', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    const data = await res.json();
    return Boolean(data.success);
  } catch {
    return false;
  }
}

export function defaultAvatarUrl(): string {
  return avatarUrlFromFilename(DEFAULT_PROFILE_AVATAR_FILENAME);
}
