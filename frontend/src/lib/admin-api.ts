import type { Me } from "./identity";

export type Group = {
  id: number;
  name: string;
  description: string;
  created_at: string;
  members?: number[];
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

export async function listUsers(): Promise<Me[]> {
  const res = await fetch("/api/admin/users", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `List users failed (${res.status})`));
  return res.json();
}

export async function adminDisableUserMFA(id: number): Promise<void> {
  const res = await fetch(`/api/admin/users/${id}/mfa/disable`, {
    method: "POST",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Disable MFA failed (${res.status})`));
  }
}

export async function setUserRole(id: number, role: "admin" | "user"): Promise<Me> {
  const res = await fetch(`/api/admin/users/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ role }),
  });
  if (!res.ok) throw new Error(await readError(res, `Patch user failed (${res.status})`));
  return res.json();
}

export async function listGroups(): Promise<Group[]> {
  const res = await fetch("/api/admin/groups", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `List groups failed (${res.status})`));
  return res.json();
}

export async function createGroup(name: string, description: string): Promise<Group> {
  const res = await fetch("/api/admin/groups", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ name, description }),
  });
  if (!res.ok) throw new Error(await readError(res, `Create group failed (${res.status})`));
  return res.json();
}

export async function deleteGroup(id: number): Promise<void> {
  const res = await fetch(`/api/admin/groups/${id}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Delete group failed (${res.status})`));
  }
}

export async function addGroupMember(groupID: number, userID: number): Promise<void> {
  const res = await fetch(`/api/admin/groups/${groupID}/members`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ user_id: userID }),
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Add member failed (${res.status})`));
  }
}

export async function removeGroupMember(groupID: number, userID: number): Promise<void> {
  const res = await fetch(`/api/admin/groups/${groupID}/members/${userID}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Remove member failed (${res.status})`));
  }
}

export type PermissionRule = {
  id: number;
  path_glob: string;
  subject_type: "anonymous" | "authenticated" | "role" | "group" | "user";
  subject_value: string;
  permission: "read" | "comment" | "write";
  created_at: string;
};

export async function listPermissions(): Promise<PermissionRule[]> {
  const res = await fetch("/api/admin/permissions", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `List permissions failed (${res.status})`));
  return res.json();
}

export async function createPermission(rule: Omit<PermissionRule, "id" | "created_at">): Promise<PermissionRule> {
  const res = await fetch("/api/admin/permissions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(rule),
  });
  if (!res.ok) throw new Error(await readError(res, `Create rule failed (${res.status})`));
  return res.json();
}

export async function deletePermission(id: number): Promise<void> {
  const res = await fetch(`/api/admin/permissions/${id}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Delete rule failed (${res.status})`));
  }
}

export async function effectivePermissions(path: string): Promise<PermissionRule[]> {
  const res = await fetch(`/api/admin/permissions/effective?path=${encodeURIComponent(path)}`, {
    credentials: "same-origin",
  });
  if (!res.ok) throw new Error(await readError(res, `Effective permissions failed (${res.status})`));
  return res.json();
}

export type GitSyncResult = {
  pulled: boolean;
  pushed: boolean;
  conflict: boolean;
  rebase_aborted: boolean;
  error?: string;
  files?: string[];
  at: string;
};

export type GitStatus = {
  remote: string;
  has_remote: boolean;
  branch: string;
  ahead: number;
  behind: number;
  rebase_in_progress: boolean;
  conflict_files?: string[];
  last_sync?: GitSyncResult;
};

export async function gitStatus(): Promise<GitStatus> {
  const res = await fetch("/api/admin/git/status", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Git status failed (${res.status})`));
  return res.json();
}

export async function gitPull(): Promise<{ status: GitStatus; conflict?: boolean; files?: string[]; error?: string; pulled?: boolean }> {
  const res = await fetch("/api/admin/git/pull", { method: "POST", credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Git pull failed (${res.status})`));
  return res.json();
}

export async function gitPush(): Promise<{ status: GitStatus; sync: GitSyncResult }> {
  const res = await fetch("/api/admin/git/push", { method: "POST", credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Git push failed (${res.status})`));
  return res.json();
}

export async function gitAbort(): Promise<{ status: GitStatus; aborted: boolean }> {
  const res = await fetch("/api/admin/git/abort", { method: "POST", credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Abort failed (${res.status})`));
  return res.json();
}

export type MailerStatus = {
  enabled: boolean;
  host: string;
  port: number;
  encryption: string;
  from_address: string;
  from_name: string;
};

export async function mailerStatus(): Promise<MailerStatus> {
  const res = await fetch("/api/admin/mailer/status", { credentials: "same-origin" });
  if (!res.ok) throw new Error(await readError(res, `Mailer status failed (${res.status})`));
  return res.json();
}

export async function sendTestMail(to?: string): Promise<{ sent: boolean; to: string }> {
  const res = await fetch("/api/admin/mailer/test", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(to ? { to } : {}),
  });
  if (!res.ok) throw new Error(await readError(res, `Test mail failed (${res.status})`));
  return res.json();
}
