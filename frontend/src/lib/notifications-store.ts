import { writable } from "svelte/store";
import * as api from "./notifications-api";
import type { AppNotification } from "./notifications-api";

const POLL_INTERVAL_MS = 60_000;

export const notifications = writable<AppNotification[]>([]);
export const unreadCount = writable<number>(0);

let timer: ReturnType<typeof setInterval> | null = null;
let inFlight = false;

export async function refreshNotifications(): Promise<void> {
  if (inFlight) return;
  inFlight = true;
  try {
    const list = await api.listNotifications();
    notifications.set(list.items);
    unreadCount.set(list.unread);
  } catch {
    // Polling is best-effort; a failed tick keeps the previous state.
  } finally {
    inFlight = false;
  }
}

// startPolling fetches immediately, then every POLL_INTERVAL_MS. Idempotent —
// CarbonShell calls it whenever a session appears.
export function startPolling(): void {
  if (timer) return;
  void refreshNotifications();
  timer = setInterval(() => void refreshNotifications(), POLL_INTERVAL_MS);
}

export function stopPolling(): void {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
  notifications.set([]);
  unreadCount.set(0);
}

// markRead flips one item optimistically, then syncs with the server.
export async function markRead(id: number): Promise<void> {
  let wasUnread = false;
  notifications.update((items) =>
    items.map((n) => {
      if (n.id !== id || n.read_at) return n;
      wasUnread = true;
      return { ...n, read_at: new Date().toISOString() };
    })
  );
  if (wasUnread) unreadCount.update((n) => Math.max(0, n - 1));
  try {
    await api.markRead(id);
  } catch {
    // Next poll re-syncs the real state.
  }
}

export async function markAllRead(): Promise<void> {
  const now = new Date().toISOString();
  notifications.update((items) => items.map((n) => (n.read_at ? n : { ...n, read_at: now })));
  unreadCount.set(0);
  try {
    await api.markAllRead();
  } catch {
    // Next poll re-syncs the real state.
  }
}
