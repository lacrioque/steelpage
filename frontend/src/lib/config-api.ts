export type ConfigFieldSchema = {
  key: string;
  type: "string" | "int" | "bool" | "enum" | "string_slice";
  enum?: string[];
  sensitive: boolean;
  read_only: boolean;
  group: string;
  order: number;
  min?: number;
  max?: number;
};

export type ConfigFieldState = {
  key: string;
  value?: unknown;
  has_value: boolean;
  has_override: boolean;
  sensitive: boolean;
  read_only: boolean;
};

export type ConfigAuditEntry = {
  id: number;
  actor_user_id: number | null;
  actor_display?: string;
  key: string;
  old_value: string | null;
  new_value: string | null;
  at: string;
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

export async function getConfigSchema(): Promise<ConfigFieldSchema[]> {
  const res = await fetch("/api/admin/config/schema", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Schema failed (${res.status})`));
  return res.json();
}

export async function getConfigEffective(): Promise<ConfigFieldState[]> {
  const res = await fetch("/api/admin/config/effective", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Effective failed (${res.status})`));
  return res.json();
}

export async function patchConfig(key: string, value: unknown): Promise<ConfigFieldState[]> {
  const res = await fetch("/api/admin/config", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ key, value }),
  });
  if (!res.ok) throw new Error(await readError(res, `Patch failed (${res.status})`));
  return res.json();
}

export async function unsetConfig(key: string): Promise<ConfigFieldState[]> {
  const res = await fetch("/api/admin/config", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ key, unset: true }),
  });
  if (!res.ok) throw new Error(await readError(res, `Unset failed (${res.status})`));
  return res.json();
}

export async function getConfigAudit(limit = 50): Promise<ConfigAuditEntry[]> {
  const res = await fetch(`/api/admin/config/audit?limit=${limit}`, { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Audit failed (${res.status})`));
  return res.json();
}

/** Trigger a browser download of the effective config as YAML. */
export function exportConfigURL(): string {
  return "/api/admin/config/export";
}
