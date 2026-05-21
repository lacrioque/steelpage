import type { SearchResult } from "./types";

async function readError(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.error === "string") return body.error;
  } catch {
    // ignore
  }
  return fallback;
}

export async function search(query: string, limit = 20): Promise<SearchResult[]> {
  const q = query.trim();
  if (!q) return [];
  const url = `/api/search?q=${encodeURIComponent(q)}&limit=${limit}`;
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) {
    throw new Error(await readError(res, `Search failed (${res.status})`));
  }
  return res.json();
}
