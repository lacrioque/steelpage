import type { ApiError, HistoryEntry, SteelpageDocument, TreeEntry } from "./types";

function apiError(status: number, message: string): ApiError {
  const err = new Error(message) as ApiError;
  err.status = status;
  return err;
}

async function readError(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.error === "string") return body.error;
  } catch {
    // ignore
  }
  return fallback;
}

export async function getDocument(path: string, ref?: string | null): Promise<SteelpageDocument> {
  const url = ref
    ? `/api/docs/${encodePath(path)}?ref=${encodeURIComponent(ref)}`
    : `/api/docs/${encodePath(path)}`;
  const res = await fetch(url);
  if (!res.ok) {
    throw apiError(res.status, await readError(res, `Failed to load document (${res.status})`));
  }
  return res.json();
}

export async function getDocumentHistory(path: string, limit = 10): Promise<HistoryEntry[]> {
  const res = await fetch(`/api/docs-history/${encodePath(path)}?limit=${limit}`);
  if (!res.ok) {
    throw apiError(res.status, await readError(res, `Failed to load history (${res.status})`));
  }
  return res.json();
}

export async function renderMarkdown(path: string, markdown: string): Promise<string> {
  const res = await fetch("/api/render", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, markdown }),
  });
  if (!res.ok) {
    throw apiError(res.status, await readError(res, `Render failed (${res.status})`));
  }
  const data = await res.json();
  return data.html as string;
}

export async function saveDocument(path: string, markdown: string): Promise<SteelpageDocument> {
  const res = await fetch(`/api/docs/${encodePath(path)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ markdown }),
  });
  if (!res.ok) {
    throw apiError(res.status, await readError(res, `Save failed (${res.status})`));
  }
  return res.json();
}

export async function getTree(): Promise<TreeEntry[]> {
  const res = await fetch("/api/tree");
  if (!res.ok) {
    throw apiError(res.status, await readError(res, `Tree failed (${res.status})`));
  }
  return res.json();
}

function encodePath(p: string): string {
  return p
    .split("/")
    .filter((s) => s.length > 0)
    .map(encodeURIComponent)
    .join("/");
}
