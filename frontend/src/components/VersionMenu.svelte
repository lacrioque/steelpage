<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { InlineLoading } from "carbon-components-svelte";
  import { getDocumentHistory } from "../lib/api";
  import { navigateToRef } from "../lib/router";
  import type { HistoryEntry } from "../lib/types";
  import { _ } from "../lib/i18n";

  export let path: string;
  export let version: string | number;
  export let sha: string;
  export let viewingRef: string | undefined = undefined;

  let open = false;
  let entries: HistoryEntry[] = [];
  let loading = false;
  let error = "";
  let triggerEl: HTMLButtonElement | null = null;

  $: shortSha = sha ? sha.slice(0, 7) : "";

  // Reload history whenever the user opens the menu for a different doc or
  // after a save (which changes `sha`).
  let lastLoadedKey = "";
  $: if (open) {
    const key = `${path}@${sha}`;
    if (key !== lastLoadedKey) {
      lastLoadedKey = key;
      void loadHistory();
    }
  }

  async function loadHistory() {
    loading = true;
    error = "";
    try {
      entries = await getDocumentHistory(path, 10);
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      loading = false;
    }
  }

  function pick(entry: HistoryEntry) {
    open = false;
    // The newest entry IS the current revision — clearing ref is more honest
    // than pinning to it (otherwise the "back to current" banner stays up).
    if (entries.length > 0 && entry.sha === entries[0].sha && !viewingRef) {
      return;
    }
    if (entries.length > 0 && entry.sha === entries[0].sha) {
      navigateToRef(null);
    } else {
      navigateToRef(entry.sha);
    }
  }

  function backToCurrent() {
    open = false;
    navigateToRef(null);
  }

  function shortDate(iso: string): string {
    try {
      return new Date(iso).toLocaleString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return iso;
    }
  }

  function onWindowClick(e: MouseEvent) {
    if (!open) return;
    const target = e.target as Node | null;
    if (triggerEl && target && triggerEl.contains(target)) return;
    // Close on any click outside the trigger; clicks on entries set open=false
    // themselves before this handler runs, so this just handles "elsewhere".
    const menu = document.querySelector(".version-menu-panel");
    if (menu && target && menu.contains(target)) return;
    open = false;
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") open = false;
  }

  onMount(() => {
    window.addEventListener("click", onWindowClick, true);
    window.addEventListener("keydown", onKeydown);
  });
  onDestroy(() => {
    window.removeEventListener("click", onWindowClick, true);
    window.removeEventListener("keydown", onKeydown);
  });
</script>

<div class="version-menu">
  <button
    bind:this={triggerEl}
    type="button"
    class="trigger"
    class:viewing-historical={!!viewingRef}
    on:click={() => (open = !open)}
    aria-haspopup="menu"
    aria-expanded={open}
  >
    {#if viewingRef}
      <span class="badge">{$_("history.viewing_historical")}</span>
    {:else}
      <span>v{version}</span>
    {/if}
    <span class="dim">·</span>
    <code>{shortSha || $_("history.uncommitted")}</code>
    <span class="caret" aria-hidden="true">▾</span>
  </button>

  {#if open}
    <div class="version-menu-panel" role="menu">
      {#if loading}
        <div class="state"><InlineLoading status="active" description={$_("history.loading")} /></div>
      {:else if error}
        <div class="state error">{error}</div>
      {:else if entries.length === 0}
        <div class="state dim">{$_("history.empty")}</div>
      {:else}
        {#if viewingRef}
          <button class="back-to-current" type="button" on:click={backToCurrent}>
            ← {$_("history.back_to_current")}
          </button>
        {/if}
        <ul>
          {#each entries as e, i (e.sha)}
            <li>
              <button
                type="button"
                class="entry"
                class:active={viewingRef ? e.sha === viewingRef : i === 0}
                on:click={() => pick(e)}
              >
                <span class="row">
                  <code class="sha">{e.sha.slice(0, 7)}</code>
                  {#if i === 0 && !viewingRef}
                    <span class="badge-current">{$_("history.current")}</span>
                  {/if}
                  <span class="dim time">{shortDate(e.date)}</span>
                </span>
                <span class="row meta">
                  <span class="author">{e.author_name}</span>
                  <span class="dim email">{e.author_email}</span>
                </span>
                {#if e.message}
                  <span class="row message">{e.message}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>

<style>
  .version-menu {
    position: relative;
    display: inline-block;
  }
  .trigger {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    background: transparent;
    border: 0;
    padding: 0.15rem 0.4rem;
    border-radius: 0.3rem;
    cursor: pointer;
    color: #6f6a60;
    font-size: 0.8rem;
    font: inherit;
  }
  .trigger:hover {
    background: rgba(0, 0, 0, 0.04);
    color: #172033;
  }
  .trigger.viewing-historical {
    color: #745700;
    background: #fff6e0;
  }
  .trigger .caret {
    font-size: 0.7rem;
    opacity: 0.7;
  }
  .trigger code {
    font-family: "JetBrains Mono", "SFMono-Regular", monospace;
    font-size: 0.78rem;
  }
  .badge {
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .version-menu-panel {
    position: absolute;
    right: 0;
    top: calc(100% + 0.3rem);
    z-index: 50;
    min-width: 320px;
    max-width: 420px;
    background: white;
    border: 1px solid #e0e0e0;
    border-radius: 0.4rem;
    box-shadow: 0 4px 18px rgba(0, 0, 0, 0.08);
    padding: 0.3rem 0;
    max-height: 60vh;
    overflow-y: auto;
  }
  .state {
    padding: 0.75rem 1rem;
    font-size: 0.9rem;
  }
  .state.error {
    color: #9b1c1c;
  }
  .state.dim {
    color: #6f6a60;
  }
  .back-to-current {
    display: block;
    width: 100%;
    text-align: left;
    background: #fff6e0;
    border: 0;
    border-bottom: 1px solid #e8d8a4;
    color: #745700;
    padding: 0.5rem 0.85rem;
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .back-to-current:hover {
    background: #ffe9b8;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .entry {
    display: block;
    width: 100%;
    text-align: left;
    background: transparent;
    border: 0;
    padding: 0.5rem 0.85rem;
    cursor: pointer;
    font: inherit;
    border-bottom: 1px solid #f4f4f4;
  }
  .entry:hover {
    background: rgba(0, 0, 0, 0.04);
  }
  .entry.active {
    background: #f4f4f4;
  }
  .entry .row {
    display: flex;
    align-items: baseline;
    gap: 0.45rem;
    font-size: 0.85rem;
  }
  .entry .row + .row {
    margin-top: 0.15rem;
  }
  .entry .sha {
    font-family: "JetBrains Mono", "SFMono-Regular", monospace;
    font-size: 0.78rem;
    color: #393939;
  }
  .entry .time {
    margin-left: auto;
    font-size: 0.78rem;
  }
  .entry .author {
    font-weight: 500;
  }
  .entry .email {
    font-size: 0.78rem;
  }
  .entry .message {
    color: #525252;
    font-size: 0.85rem;
  }
  .badge-current {
    font-size: 0.7rem;
    background: #defbe6;
    color: #105a2b;
    padding: 0 0.4rem;
    border-radius: 999px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-weight: 600;
  }
  .dim {
    color: #6f6a60;
  }
</style>
