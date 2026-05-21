import { writable, get } from "svelte/store";
import { logout as apiLogout } from "./auth-api";
import { applyFont } from "./fonts";

export type Me = {
  id: number;
  email: string | null;
  display_name: string;
  role: "admin" | "user";
  groups: string[] | null;
  oidc_provider?: string | null;
  oidc_subject?: string | null;
  created_at: string;
  email_verified_at?: string | null;
  totp_enabled_at?: string | null;
  font_family?: string | null;
};

export const me = writable<Me | null>(null);
export const meLoaded = writable<boolean>(false);

// Keep the document's --steelpage-font in sync with the current user's
// preference. Signed-out users fall back to the default IBM Plex stack.
me.subscribe((u) => applyFont(u?.font_family ?? null));

export async function refreshMe(): Promise<Me | null> {
  const res = await fetch("/api/me", { credentials: "same-origin" });
  if (res.status === 204) {
    me.set(null);
  } else if (res.ok) {
    me.set(await res.json());
  } else {
    me.set(null);
  }
  meLoaded.set(true);
  return get(me);
}

export async function logout(): Promise<void> {
  await apiLogout();
  me.set(null);
}

export function setMe(next: Me | null): void {
  me.set(next);
  meLoaded.set(true);
}

// setFontFamily PATCHes /api/me with the new preference. Pass null to clear
// the override and revert to the default IBM Plex Sans.
export async function setFontFamily(font: string | null): Promise<Me> {
  const res = await fetch("/api/me", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ font_family: font }),
  });
  if (!res.ok) {
    let msg = `Failed to save preference (${res.status})`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      // ignore
    }
    throw new Error(msg);
  }
  const updated: Me = await res.json();
  me.set(updated);
  return updated;
}
