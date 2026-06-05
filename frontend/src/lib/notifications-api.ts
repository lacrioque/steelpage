export type NotificationKind = "mention" | "reply";

export type AppNotification = {
  id: number;
  kind: NotificationKind;
  path: string;
  comment_id: number;
  actor: { id: number; display_name: string };
  created_at: string;
  read_at?: string | null;
};

export type NotificationList = {
  items: AppNotification[];
  unread: number;
};

export type MentionableUser = {
  id: number;
  display_name: string;
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

export async function listNotifications(): Promise<NotificationList> {
  const res = await fetch("/api/notifications", { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error(await readError(res, `List notifications failed (${res.status})`));
  }
  return res.json();
}

export async function markRead(id: number): Promise<void> {
  const res = await fetch(`/api/notifications/${id}`, {
    method: "PATCH",
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Mark notification read failed (${res.status})`));
  }
}

export async function markAllRead(): Promise<void> {
  const res = await fetch("/api/notifications/read-all", {
    method: "POST",
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Mark all read failed (${res.status})`));
  }
}

export async function listMentionable(path: string): Promise<MentionableUser[]> {
  const res = await fetch(`/api/users/mentionable?path=${encodeURIComponent(path)}`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error(await readError(res, `List mentionable users failed (${res.status})`));
  }
  return res.json();
}
