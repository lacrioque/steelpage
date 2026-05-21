<script lang="ts">
  import {
    OverflowMenu,
    OverflowMenuItem,
    Modal,
    TextInput,
    InlineNotification,
  } from "carbon-components-svelte";
  import { deleteDocument, moveDocument, copyDocument } from "../lib/docs-api";
  import { navigateToDoc } from "../lib/router";
  import { forceReload } from "../lib/document-store";
  import { _ } from "../lib/i18n";

  export let path: string;

  let moveOpen = false;
  let copyOpen = false;
  let deleteOpen = false;
  let moveTarget = "";
  let copyTarget = "";
  let busy = false;
  let error = "";

  function openMove() {
    moveTarget = path;
    error = "";
    moveOpen = true;
  }
  function openCopy() {
    copyTarget = suggestCopy(path);
    error = "";
    copyOpen = true;
  }
  function openDelete() {
    error = "";
    deleteOpen = true;
  }

  function suggestCopy(p: string): string {
    if (p.endsWith(".md")) return p.replace(/\.md$/i, "-copy.md");
    return `${p}-copy`;
  }

  async function submitMove() {
    if (!moveTarget || moveTarget === path) return;
    busy = true;
    error = "";
    try {
      await moveDocument(path, moveTarget);
      moveOpen = false;
      navigateToDoc(moveTarget);
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      busy = false;
    }
  }

  async function submitCopy() {
    if (!copyTarget || copyTarget === path) return;
    busy = true;
    error = "";
    try {
      await copyDocument(path, copyTarget);
      copyOpen = false;
      navigateToDoc(copyTarget);
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      busy = false;
    }
  }

  async function submitDelete() {
    busy = true;
    error = "";
    try {
      await deleteDocument(path);
      deleteOpen = false;
      navigateToDoc("README.md");
      forceReload("README.md");
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      busy = false;
    }
  }
</script>

<div class="file-actions">
  <OverflowMenu flipped iconDescription={$_("file_actions.menu")}>
    <OverflowMenuItem text={$_("file_actions.move")} on:click={openMove} />
    <OverflowMenuItem text={$_("file_actions.copy")} on:click={openCopy} />
    <OverflowMenuItem danger text={$_("file_actions.delete")} on:click={openDelete} />
  </OverflowMenu>
</div>

<Modal
  bind:open={moveOpen}
  modalHeading={$_("file_actions.move_heading", { values: { path } })}
  primaryButtonText={busy ? $_("file_actions.working") : $_("file_actions.move_submit")}
  secondaryButtonText={$_("file_actions.cancel")}
  primaryButtonDisabled={busy || !moveTarget || moveTarget === path}
  on:submit={submitMove}
  on:click:button--secondary={() => (moveOpen = false)}
>
  <TextInput
    labelText={$_("file_actions.new_path")}
    placeholder={$_("file_actions.path_placeholder")}
    bind:value={moveTarget}
  />
  {#if error}
    <InlineNotification kind="error" title={$_("file_actions.error")} subtitle={error} lowContrast hideCloseButton />
  {/if}
</Modal>

<Modal
  bind:open={copyOpen}
  modalHeading={$_("file_actions.copy_heading", { values: { path } })}
  primaryButtonText={busy ? $_("file_actions.working") : $_("file_actions.copy_submit")}
  secondaryButtonText={$_("file_actions.cancel")}
  primaryButtonDisabled={busy || !copyTarget || copyTarget === path}
  on:submit={submitCopy}
  on:click:button--secondary={() => (copyOpen = false)}
>
  <TextInput
    labelText={$_("file_actions.dest_path")}
    placeholder={$_("file_actions.path_placeholder")}
    bind:value={copyTarget}
  />
  {#if error}
    <InlineNotification kind="error" title={$_("file_actions.error")} subtitle={error} lowContrast hideCloseButton />
  {/if}
</Modal>

<Modal
  bind:open={deleteOpen}
  danger
  modalHeading={$_("file_actions.delete_heading", { values: { path } })}
  primaryButtonText={busy ? $_("file_actions.working") : $_("file_actions.delete_submit")}
  secondaryButtonText={$_("file_actions.cancel")}
  primaryButtonDisabled={busy}
  on:submit={submitDelete}
  on:click:button--secondary={() => (deleteOpen = false)}
>
  <p>{$_("file_actions.delete_warning")}</p>
  {#if error}
    <InlineNotification kind="error" title={$_("file_actions.error")} subtitle={error} lowContrast hideCloseButton />
  {/if}
</Modal>

<style>
  .file-actions {
    display: inline-flex;
  }
</style>
