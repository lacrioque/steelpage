import type { SteelpageDocument } from "./types";

function encodePath(p: string): string {
  return p
    .split("/")
    .filter((s) => s.length > 0)
    .map(encodeURIComponent)
    .join("/");
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

export async function deleteDocument(path: string): Promise<void> {
  const res = await fetch(`/api/docs/${encodePath(path)}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(await readError(res, `Delete failed (${res.status})`));
  }
}

export async function moveDocument(from: string, to: string): Promise<SteelpageDocument> {
  const res = await fetch("/api/docs-move", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ from, to }),
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Move failed (${res.status})`));
  }
  return res.json();
}

export async function copyDocument(from: string, to: string): Promise<SteelpageDocument> {
  const res = await fetch("/api/docs-copy", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ from, to }),
  });
  if (!res.ok) {
    throw new Error(await readError(res, `Copy failed (${res.status})`));
  }
  return res.json();
}
