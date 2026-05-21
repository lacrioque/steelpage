import type { Comment, CommentStatus } from "./types";

async function readError(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.error === "string") return body.error;
  } catch {
    // ignore
  }
  return fallback;
}

export async function listComments(path: string): Promise<Comment[]> {
  const res = await fetch(`/api/comments?path=${encodeURIComponent(path)}`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error(await readError(res, `List comments failed (${res.status})`));
  }
  return res.json();
}

export type CreateCommentInput = {
  path: string;
  line_start: number;
  line_end: number;
  anchor_text: string;
  body: string;
  reply_to?: number | null;
};

export async function createComment(input: CreateCommentInput): Promise<Comment> {
  const res = await fetch("/api/comments", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Create comment failed (${res.status})`));
  }
  return res.json();
}

export async function updateComment(
  id: number,
  patch: { body?: string; status?: CommentStatus }
): Promise<Comment> {
  const res = await fetch(`/api/comments/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(patch),
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Update comment failed (${res.status})`));
  }
  return res.json();
}
