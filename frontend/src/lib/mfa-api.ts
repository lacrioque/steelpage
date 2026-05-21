import type { Me } from "./identity";

export type MFASetupChallenge = {
  secret: string;
  otpauth_url: string;
  qr_png: string;
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

export async function mfaSetupStart(): Promise<MFASetupChallenge> {
  const res = await fetch("/api/auth/mfa/setup-start", {
    method: "POST",
    credentials: "same-origin",
  });
  if (!res.ok) throw new Error(await readError(res, `Setup failed (${res.status})`));
  return res.json();
}

export async function mfaSetupConfirm(code: string): Promise<Me> {
  const res = await fetch("/api/auth/mfa/setup-confirm", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ code }),
  });
  if (!res.ok) throw new Error(await readError(res, `Confirm failed (${res.status})`));
  return res.json();
}

export async function mfaDisable(code: string): Promise<void> {
  const res = await fetch("/api/auth/mfa/disable", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ code }),
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Disable failed (${res.status})`));
  }
}

export async function mfaLogin(code: string): Promise<Me> {
  const res = await fetch("/api/auth/login/mfa", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ code }),
  });
  if (!res.ok) throw new Error(await readError(res, `Login failed (${res.status})`));
  return res.json();
}
