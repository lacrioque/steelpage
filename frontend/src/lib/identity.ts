import { writable, get } from "svelte/store";
import { logout as apiLogout } from "./auth-api";

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
};

export const me = writable<Me | null>(null);
export const meLoaded = writable<boolean>(false);

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
