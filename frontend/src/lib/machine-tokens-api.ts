export type MachineToken = {
  id: number;
  user_id: number;
  name: string;
  display_name: string;
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

export async function listMachineTokens(): Promise<MachineToken[]> {
  const res = await fetch("/api/admin/machine-tokens", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `List machine tokens failed (${res.status})`));
  return res.json();
}

export type CreateMachineTokenInput = {
  name: string;
  scopes: string[];
  expires_at?: string | null;
};

export async function createMachineToken(input: CreateMachineTokenInput): Promise<MachineToken> {
  const res = await fetch("/api/admin/machine-tokens", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(await readError(res, `Create machine token failed (${res.status})`));
  return res.json();
}

export async function revokeMachineToken(id: number): Promise<void> {
  const res = await fetch(`/api/admin/machine-tokens/${id}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Revoke machine token failed (${res.status})`));
  }
}
