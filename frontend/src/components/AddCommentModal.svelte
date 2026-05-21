<script lang="ts">
  import { Modal, TextArea } from "carbon-components-svelte";
  import { addComment } from "../lib/comments-store";
  import { me } from "../lib/identity";
  import { _ } from "../lib/i18n";

  export let open = false;
  export let path = "";
  export let lineNumber = 1;
  export let anchorText = "";
  export let replyTo: number | null = null;
  export let replyToAuthor = "";

  let body = "";
  let busy = false;
  let error = "";

  $: if (open) {
    body = "";
    error = "";
  }

  async function submit() {
    const trimmed = body.trim();
    if (!trimmed) {
      error = $_("add_comment.empty_error");
      return;
    }
    if (!$me) {
      error = $_("add_comment.no_identity");
      return;
    }
    busy = true;
    error = "";
    try {
      await addComment({
        path,
        line_start: lineNumber,
        line_end: lineNumber,
        anchor_text: anchorText,
        body: trimmed,
        reply_to: replyTo,
      });
      open = false;
    } catch (err) {
      error = err instanceof Error ? err.message : $_("add_comment.failed");
    } finally {
      busy = false;
    }
  }
</script>

<Modal
  bind:open
  modalHeading={replyTo
    ? $_("add_comment.heading_reply", { values: { author: replyToAuthor || "comment" } })
    : $_("add_comment.heading", { values: { line: lineNumber } })}
  primaryButtonText={busy ? $_("add_comment.posting") : $_("add_comment.post")}
  secondaryButtonText={$_("add_comment.cancel")}
  primaryButtonDisabled={busy || body.trim().length === 0}
  on:submit={submit}
  on:click:button--secondary={() => (open = false)}
>
  <p style="margin-bottom:1rem;color:#525252;font-family:monospace;font-size:0.85rem;background:#f4f4f4;padding:0.5rem;border-radius:0.25rem;overflow-x:auto;white-space:pre">{anchorText || $_("add_comment.empty_line")}</p>
  <TextArea
    labelText={$_("add_comment.label")}
    placeholder={$_("add_comment.placeholder")}
    bind:value={body}
    invalid={!!error}
    invalidText={error}
    rows={4}
  />
</Modal>
