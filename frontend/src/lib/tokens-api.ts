export type ApiToken = {
  id: number;
  user_id: number;
  name: string;
  scopes: string[];
  expires_at: string | null;
  last_used_at: string | null;
  created_at: string;
  plaintext?: string; // only present on the create response
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

export async function listTokens(): Promise<ApiToken[]> {
  const res = await fetch("/api/me/tokens", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `List tokens failed (${res.status})`));
  return res.json();
}

export type CreateTokenInput = {
  name: string;
  scopes: string[];
  expires_at?: string | null;
};

export async function createToken(input: CreateTokenInput): Promise<ApiToken> {
  const res = await fetch("/api/me/tokens", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(await readError(res, `Create token failed (${res.status})`));
  return res.json();
}

export async function revokeToken(id: number): Promise<void> {
  const res = await fetch(`/api/me/tokens/${id}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Revoke token failed (${res.status})`));
  }
}
