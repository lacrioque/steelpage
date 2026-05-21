<script lang="ts">
  import { saveDocument } from "../lib/api";
  import { _ } from "../lib/i18n";

  export let path: string;
  export let onCreated: () => void;

  let busy = false;
  let error = "";

  async function create() {
    busy = true;
    error = "";
    try {
      const body = $_("not_found.default_body", { values: { title: defaultTitle(path) } });
      await saveDocument(path, body);
      onCreated();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      busy = false;
    }
  }

  function defaultTitle(p: string): string {
    const last = p.split("/").pop() ?? "untitled";
    return last.replace(/\.md$/i, "").replace(/[-_]/g, " ");
  }
</script>

<section class="not-found">
  <h1>{$_("not_found.title")}</h1>
  <p>{$_("not_found.explanation", { values: { path } })}</p>
  <button type="button" on:click={create} disabled={busy}>
    {busy ? $_("not_found.creating") : $_("not_found.create")}
  </button>
  {#if error}<p class="error">{error}</p>{/if}
</section>

<style>
  .not-found {
    max-width: 540px;
    margin: 6rem auto;
    padding: 0 1.25rem;
    text-align: center;
    color: #172033;
  }
  .not-found h1 {
    margin-bottom: 0.4rem;
  }
  .not-found button {
    margin-top: 1.5rem;
    border: 1px solid #cfc7b8;
    border-radius: 999px;
    background: white;
    padding: 0.5rem 1rem;
    cursor: pointer;
    font: inherit;
  }
  .not-found button[disabled] {
    opacity: 0.5;
    cursor: progress;
  }
  .error {
    color: #9b1c1c;
    margin-top: 1rem;
  }
</style>
