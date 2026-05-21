<script lang="ts">
  import { ComposedModal, ModalHeader, ModalBody, Search as CarbonSearch, InlineLoading } from "carbon-components-svelte";
  import { search } from "../lib/search-api";
  import { navigateToDoc } from "../lib/router";
  import type { SearchResult } from "../lib/types";
  import { _ } from "../lib/i18n";

  export let open = false;

  let query = "";
  let results: SearchResult[] = [];
  let busy = false;
  let error = "";
  let timer: ReturnType<typeof setTimeout> | null = null;

  $: if (!open) {
    query = "";
    results = [];
    error = "";
    busy = false;
  }

  function onInput() {
    if (timer) clearTimeout(timer);
    error = "";
    const q = query.trim();
    if (!q) {
      results = [];
      busy = false;
      return;
    }
    busy = true;
    timer = setTimeout(async () => {
      try {
        results = await search(q, 20);
      } catch (err) {
        results = [];
        error = err instanceof Error ? err.message : "";
      } finally {
        busy = false;
      }
    }, 200);
  }

  function pick(r: SearchResult) {
    navigateToDoc(r.path);
    open = false;
  }

  function pickFirst() {
    if (results.length > 0) pick(results[0]);
  }
</script>

<ComposedModal bind:open size="lg" on:submit={pickFirst}>
  <ModalHeader title={$_("search.heading")} label={$_("search.shortcut_hint")} />
  <ModalBody hasForm>
    <CarbonSearch
      labelText={$_("search.placeholder")}
      placeholder={$_("search.placeholder")}
      autofocus
      bind:value={query}
      on:input={onInput}
    />

    <div class="results" role="listbox">
      {#if busy}
        <div class="loading"><InlineLoading status="active" description={$_("comments.loading")} /></div>
      {:else if error}
        <p class="error">{$_("search.failed", { values: { error } })}</p>
      {:else if query.trim() === ""}
        <p class="hint">{$_("search.type_to_search")}</p>
      {:else if results.length === 0}
        <p class="hint">{$_("search.no_results", { values: { query } })}</p>
      {:else}
        {#each results as r (r.path)}
          <button
            type="button"
            class="row"
            on:click={() => pick(r)}
          >
            <div class="title">{r.title || r.path}</div>
            <div class="path">{r.path}</div>
            {#if r.heading_snippet}
              <div class="snippet headings">{@html r.heading_snippet}</div>
            {/if}
            {#if r.body_snippet}
              <div class="snippet body">{@html r.body_snippet}</div>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  </ModalBody>
</ComposedModal>

<style>
  .results {
    margin-top: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    max-height: 60vh;
    overflow-y: auto;
  }
  .row {
    display: block;
    width: 100%;
    text-align: left;
    background: white;
    border: 1px solid #e0e0e0;
    border-radius: 0.4rem;
    padding: 0.75rem 1rem;
    cursor: pointer;
    font: inherit;
    color: inherit;
  }
  .row:hover {
    background: #f4f4f4;
    border-color: #c6c6c6;
  }
  .row:focus-visible {
    outline: 2px solid #0f62fe;
    outline-offset: 2px;
  }
  .row .title {
    font-weight: 600;
    margin-bottom: 0.15rem;
  }
  .row .path {
    color: #6f6a60;
    font-size: 0.8rem;
    font-family: "JetBrains Mono", monospace;
    margin-bottom: 0.35rem;
  }
  .row .snippet {
    color: #393939;
    font-size: 0.88rem;
    line-height: 1.4;
  }
  .row .snippet.headings {
    color: #525252;
    font-style: italic;
    margin-bottom: 0.2rem;
  }
  .row :global(mark) {
    background: #fff1c2;
    color: inherit;
    padding: 0 0.15rem;
    border-radius: 0.15rem;
  }
  .hint,
  .loading {
    padding: 1.5rem 0;
    color: #6f6a60;
    text-align: center;
  }
  .error {
    color: #9b1c1c;
    padding: 0.5rem 0;
  }
</style>
