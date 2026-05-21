import { writable, get } from "svelte/store";
import type { ApiError, SteelpageDocument } from "./types";
import { getDocument, renderMarkdown, saveDocument } from "./api";
import * as commentsStore from "./comments-store";
import { _ } from "./i18n";

export type SaveState = "idle" | "saving" | "saved" | "error";

export const doc = writable<SteelpageDocument | null>(null);
export const loading = writable<boolean>(true);
export const error = writable<string>("");
export const notFoundPath = writable<string>("");
export const editing = writable<boolean>(false);
export const draft = writable<string>("");
export const previewHtml = writable<string>("");
export const saveState = writable<SaveState>("idle");
export const saveError = writable<string>("");

let lastLoadKey = "";
let previewTimer: ReturnType<typeof setTimeout> | null = null;
let savedTimer: ReturnType<typeof setTimeout> | null = null;

// load fetches a document. `ref` (optional sha) loads a historical revision —
// editing is force-disabled in that case so users can't accidentally try to
// save back to history.
export async function load(path: string, ref?: string | null): Promise<void> {
  const key = `${path}@${ref ?? ""}`;
  if (key === lastLoadKey) return;
  lastLoadKey = key;
  loading.set(true);
  error.set("");
  notFoundPath.set("");
  doc.set(null);
  editing.set(false);
  saveState.set("idle");
  saveError.set("");

  try {
    const next = await getDocument(path, ref ?? null);
    doc.set(next);
    draft.set(next.markdown);
    previewHtml.set(next.html);
    void commentsStore.loadForPath(path);
  } catch (err) {
    const apiErr = err as ApiError;
    if (apiErr.status === 404) {
      notFoundPath.set(path);
    } else {
      error.set(apiErr.message ?? "Failed to load document");
    }
  } finally {
    loading.set(false);
  }
}

export function forceReload(path: string, ref?: string | null): void {
  lastLoadKey = "";
  void load(path, ref);
}

export function toggleEdit(): void {
  editing.update((cur) => !cur);
}

export function setDraft(next: string): void {
  draft.set(next);
  schedulePreview();
}

function schedulePreview(): void {
  const d = get(doc);
  if (!d) return;
  if (previewTimer) clearTimeout(previewTimer);
  previewTimer = setTimeout(async () => {
    try {
      const html = await renderMarkdown(d.path, get(draft));
      previewHtml.set(html);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      const t = get(_);
      previewHtml.set(`<p class="error">${escapeHtml(t("document.preview_failed", { values: { error: msg } }))}</p>`);
    }
  }, 250);
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (ch) => {
    switch (ch) {
      case "&":
        return "&amp;";
      case "<":
        return "&lt;";
      case ">":
        return "&gt;";
      case "\"":
        return "&quot;";
      default:
        return "&#39;";
    }
  });
}

export async function save(): Promise<void> {
  const d = get(doc);
  if (!d) return;
  saveState.set("saving");
  saveError.set("");
  try {
    const updated = await saveDocument(d.path, get(draft));
    doc.set(updated);
    draft.set(updated.markdown);
    previewHtml.set(updated.html);
    saveState.set("saved");
    commentsStore.refreshAfterSave();
    if (savedTimer) clearTimeout(savedTimer);
    savedTimer = setTimeout(() => {
      if (get(saveState) === "saved") saveState.set("idle");
    }, 2000);
  } catch (err) {
    saveState.set("error");
    saveError.set(err instanceof Error ? err.message : "Unknown error");
  }
}
