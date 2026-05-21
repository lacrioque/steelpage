<script lang="ts">
  import { onMount } from "svelte";
  import { Button, Modal, TextInput, TextArea, InlineNotification } from "carbon-components-svelte";
  import Add from "carbon-icons-svelte/lib/Add.svelte";
  import CollapseAll from "carbon-icons-svelte/lib/CollapseAll.svelte";
  import ExpandAll from "carbon-icons-svelte/lib/ExpandAll.svelte";
  import { currentDoc, navigateToDoc } from "../lib/router";
  import { getTree, saveDocument } from "../lib/api";
  import type { TreeEntry } from "../lib/types";
  import { me } from "../lib/identity";
  import { _ } from "../lib/i18n";

  type Node = {
    name: string;
    path: string;
    children: Map<string, Node>;
    isFile: boolean;
  };

  let entries: TreeEntry[] = [];
  let error = "";
  let loading = true;

  let addOpen = false;
  let addPath = "";
  let addBody = "";
  let addBusy = false;
  let addError = "";

  $: active = $currentDoc;

  async function refreshTree() {
    try {
      entries = await getTree();
    } catch (err) {
      error = err instanceof Error ? err.message : $_("tree.error");
    } finally {
      loading = false;
    }
  }

  onMount(refreshTree);

  function openAdd() {
    addPath = "";
    addBody = "";
    addError = "";
    addOpen = true;
  }

  async function submitAdd() {
    const target = addPath.trim().replace(/^\/+/, "");
    if (!target) return;
    addBusy = true;
    addError = "";
    try {
      const body = addBody.trim() === "" ? `# ${defaultTitle(target)}\n\n` : addBody;
      await saveDocument(target, body);
      addOpen = false;
      await refreshTree();
      navigateToDoc(target);
    } catch (err) {
      addError = err instanceof Error ? err.message : "";
    } finally {
      addBusy = false;
    }
  }

  function defaultTitle(p: string): string {
    const last = p.split("/").pop() ?? "untitled";
    return last.replace(/\.md$/i, "").replace(/[-_]/g, " ");
  }

  // Toggle every <details> in the tree at once. Imperative DOM access is
  // fine here — the tree is rebuilt from the `entries` store on each
  // refresh, so we don't need reactive state for collapsed-ness.
  function setAllOpen(open: boolean) {
    const root = document.querySelector(".archive-tree");
    if (!root) return;
    root.querySelectorAll("details").forEach((d) => {
      (d as HTMLDetailsElement).open = open;
    });
  }

  $: hasFolders = entries.some((e) => e.path.includes("/"));

  function buildTree(items: TreeEntry[]): Node {
    const root: Node = { name: "", path: "", children: new Map(), isFile: false };
    for (const item of items) {
      const segments = item.path.split("/").filter(Boolean);
      let cursor = root;
      segments.forEach((seg, idx) => {
        if (!cursor.children.has(seg)) {
          const childPath = segments.slice(0, idx + 1).join("/");
          cursor.children.set(seg, {
            name: seg,
            path: childPath,
            children: new Map(),
            isFile: false,
          });
        }
        cursor = cursor.children.get(seg)!;
        if (idx === segments.length - 1) cursor.isFile = true;
      });
    }
    return root;
  }

  function sortChildren(node: Node): Node[] {
    const arr = [...node.children.values()];
    arr.sort((a, b) => {
      // Folders first, then files; both alphabetical
      if (a.isFile !== b.isFile) return a.isFile ? 1 : -1;
      return a.name.localeCompare(b.name);
    });
    return arr;
  }

  function onClick(e: MouseEvent, node: Node) {
    if (!node.isFile) return;
    e.preventDefault();
    navigateToDoc(node.path);
  }

  $: tree = buildTree(entries);
</script>

<aside class="archive-tree">
  <header>
    <span class="brand">
      <img src="/logo.svg" alt="" width="20" height="20" />
      <span>{$_("tree.header")}</span>
    </span>
    <div class="header-actions">
      {#if hasFolders}
        <Button
          kind="ghost"
          size="sm"
          icon={CollapseAll}
          iconDescription={$_("tree.collapse_all")}
          tooltipPosition="bottom"
          tooltipAlignment="end"
          on:click={() => setAllOpen(false)}
        />
        <Button
          kind="ghost"
          size="sm"
          icon={ExpandAll}
          iconDescription={$_("tree.expand_all")}
          tooltipPosition="bottom"
          tooltipAlignment="end"
          on:click={() => setAllOpen(true)}
        />
      {/if}
      {#if $me}
        <Button
          kind="ghost"
          size="sm"
          icon={Add}
          iconDescription={$_("tree.add_file")}
          tooltipPosition="bottom"
          tooltipAlignment="end"
          on:click={openAdd}
        />
      {/if}
    </div>
  </header>
  {#if loading}
    <p class="muted">{$_("tree.loading")}</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if entries.length === 0}
    <p class="muted">{$_("tree.empty")}</p>
  {:else}
    {@const top = sortChildren(tree)}
    <ul>
      {#each top as node (node.path)}
        {@render branch(node)}
      {/each}
    </ul>
  {/if}
</aside>

<Modal
  bind:open={addOpen}
  modalHeading={$_("tree.add_file_heading")}
  primaryButtonText={addBusy ? $_("tree.add_busy") : $_("tree.add_submit")}
  secondaryButtonText={$_("tree.add_cancel")}
  primaryButtonDisabled={addBusy || !addPath.trim()}
  on:submit={submitAdd}
  on:click:button--secondary={() => (addOpen = false)}
>
  <p style="color:#525252;margin-bottom:1rem">{$_("tree.add_help")}</p>
  <TextInput
    labelText={$_("tree.add_path_label")}
    placeholder={$_("tree.add_path_placeholder")}
    bind:value={addPath}
  />
  <div style="margin-top:1rem">
    <TextArea
      labelText={$_("tree.add_body_label")}
      placeholder={$_("tree.add_body_placeholder")}
      helperText={$_("tree.add_body_help")}
      bind:value={addBody}
      rows={4}
    />
  </div>
  {#if addError}
    <div style="margin-top:1rem">
      <InlineNotification kind="error" title={$_("tree.add_error")} subtitle={addError} lowContrast hideCloseButton />
    </div>
  {/if}
</Modal>

{#snippet branch(node: Node)}
  <li>
    {#if node.isFile}
      <a
        href={`/docs/${node.path}`}
        class:active={active === node.path}
        on:click={(e) => onClick(e, node)}
      >
        {node.name}
      </a>
    {:else}
      <details open>
        <summary>{node.name}</summary>
        <ul>
          {#each sortChildren(node) as child (child.path)}
            {@render branch(child)}
          {/each}
        </ul>
      </details>
    {/if}
  </li>
{/snippet}

<style>
  .archive-tree {
    padding: 1rem 0.75rem;
    border-right: 1px solid #ded8cc;
    overflow-y: auto;
    background: rgba(255, 255, 255, 0.5);
  }
  .archive-tree header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #6f6a60;
    padding: 0 0.25rem 0.5rem;
  }
  .archive-tree header .brand {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }
  .archive-tree header .brand img {
    display: block;
  }
  .archive-tree header .header-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.1rem;
  }
  .archive-tree ul {
    list-style: none;
    margin: 0;
    padding-left: 0.75rem;
  }
  .archive-tree > ul {
    padding-left: 0;
  }
  .archive-tree a,
  .archive-tree summary {
    display: block;
    padding: 0.25rem 0.4rem;
    color: #172033;
    text-decoration: none;
    border-radius: 0.4rem;
    cursor: pointer;
    font-size: 0.92rem;
  }
  .archive-tree a:hover,
  .archive-tree summary:hover {
    background: rgba(0, 0, 0, 0.04);
  }
  .archive-tree a.active {
    background: #172033;
    color: white;
  }
  .archive-tree details summary::-webkit-details-marker {
    display: none;
  }
  .archive-tree details summary::before {
    content: "▸ ";
    color: #6f6a60;
  }
  .archive-tree details[open] summary::before {
    content: "▾ ";
  }
  .muted {
    color: #6f6a60;
    padding: 0.25rem 0.5rem;
  }
  .error {
    color: #9b1c1c;
    padding: 0.25rem 0.5rem;
  }
</style>
