import type { Me } from "./identity";

export type AuthProvider = {
  name: string;
  label: string;
};

export type AuthCapabilities = {
  local_enabled: boolean;
  allow_anonymous_read: boolean;
  providers: AuthProvider[];
};

async function readError(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.error === "string") return body.error;
  } catch {
    // ignore
  }
  return fallback;
}

export async function getCapabilities(): Promise<AuthCapabilities> {
  const res = await fetch("/api/auth/providers", { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error(await readError(res, `Capabilities failed (${res.status})`));
  }
  return res.json();
}

export async function register(input: { email: string; password: string; display_name: string }): Promise<Me> {
  const res = await fetch("/api/auth/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Register failed (${res.status})`));
  }
  return res.json();
}

export type LoginResult =
  | { kind: "user"; user: Me }
  | { kind: "mfa_required" };

export async function login(email: string, password: string): Promise<LoginResult> {
  const res = await fetch("/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Login failed (${res.status})`));
  }
  const body = await res.json();
  if (body && body.mfa_required === true) {
    return { kind: "mfa_required" };
  }
  return { kind: "user", user: body };
}

export async function logout(): Promise<void> {
  const res = await fetch("/api/auth/logout", {
    method: "POST",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Logout failed (${res.status})`));
  }
}
