<script lang="ts">
  import { tick } from "svelte";
  import { Modal, TextArea } from "carbon-components-svelte";
  import { addComment } from "../lib/comments-store";
  import { listMentionable, type MentionableUser } from "../lib/notifications-api";
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

  // @mention autocomplete state. Users load lazily the first time the modal
  // opens for a path; suggestions track the "@query" word under the caret.
  let mentionable: MentionableUser[] = [];
  let mentionableFor = "";
  let textareaRef: HTMLTextAreaElement | null = null;
  let suggestions: MentionableUser[] = [];
  let suggestIndex = 0;
  let mentionStart = -1; // index of the '@' in body, -1 = no active mention

  $: if (open) {
    body = "";
    error = "";
    closeSuggestions();
    void loadMentionable();
  }

  async function loadMentionable() {
    if (mentionableFor === path && mentionable.length > 0) return;
    try {
      mentionable = await listMentionable(path);
      mentionableFor = path;
    } catch {
      mentionable = []; // autocomplete simply stays silent
    }
  }

  function closeSuggestions() {
    suggestions = [];
    suggestIndex = 0;
    mentionStart = -1;
  }

  // isBoundary mirrors the backend rule: '@' only opens a mention at the
  // start of the text or after whitespace/punctuation, so e-mail addresses
  // typed into a comment don't trigger the dropdown.
  function isBoundary(ch: string): boolean {
    return /[\s\p{P}\p{S}]/u.test(ch);
  }

  function updateSuggestions() {
    if (!textareaRef || mentionable.length === 0) return;
    const caret = textareaRef.selectionStart ?? body.length;
    const before = body.slice(0, caret);
    const at = before.lastIndexOf("@");
    if (at === -1 || (at > 0 && !isBoundary(before[at - 1]))) {
      closeSuggestions();
      return;
    }
    const query = before.slice(at + 1);
    if (query.includes("\n")) {
      closeSuggestions();
      return;
    }
    const q = query.toLowerCase();
    const matches = mentionable
      .filter((u) => u.display_name.toLowerCase().startsWith(q) && u.id !== $me?.id)
      .slice(0, 6);
    if (matches.length === 0) {
      closeSuggestions();
      return;
    }
    suggestions = matches;
    suggestIndex = 0;
    mentionStart = at;
  }

  async function pick(u: MentionableUser) {
    if (!textareaRef || mentionStart < 0) return;
    const caret = textareaRef.selectionStart ?? body.length;
    const inserted = `@${u.display_name} `;
    body = body.slice(0, mentionStart) + inserted + body.slice(caret);
    const nextCaret = mentionStart + inserted.length;
    closeSuggestions();
    await tick();
    textareaRef.focus();
    textareaRef.setSelectionRange(nextCaret, nextCaret);
  }

  function onKeydown(e: KeyboardEvent) {
    if (suggestions.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      suggestIndex = (suggestIndex + 1) % suggestions.length;
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      suggestIndex = (suggestIndex - 1 + suggestions.length) % suggestions.length;
    } else if (e.key === "Enter" || e.key === "Tab") {
      e.preventDefault();
      e.stopPropagation();
      void pick(suggestions[suggestIndex]);
    } else if (e.key === "Escape") {
      e.stopPropagation();
      closeSuggestions();
    }
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
  <div class="composer">
    <TextArea
      labelText={$_("add_comment.label")}
      placeholder={$_("add_comment.placeholder")}
      helperText={$_("add_comment.mention_hint")}
      bind:value={body}
      bind:ref={textareaRef}
      invalid={!!error}
      invalidText={error}
      rows={4}
      on:input={updateSuggestions}
      on:click={updateSuggestions}
      on:keydown={onKeydown}
      on:blur={() => setTimeout(closeSuggestions, 150)}
    />
    {#if suggestions.length > 0}
      <ul class="mention-suggest" role="listbox">
        {#each suggestions as u, i (u.id)}
          <li role="option" aria-selected={i === suggestIndex}>
            <button
              type="button"
              class:active={i === suggestIndex}
              on:mousedown|preventDefault={() => void pick(u)}
            >
              @{u.display_name}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</Modal>

<style>
  .composer {
    position: relative;
  }
  .mention-suggest {
    position: absolute;
    left: 0;
    right: 0;
    margin: 0;
    padding: 0;
    list-style: none;
    background: #ffffff;
    border: 1px solid #e0e0e0;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
    z-index: 9100; /* above the Carbon modal */
    max-height: 12rem;
    overflow-y: auto;
  }
  .mention-suggest button {
    display: block;
    width: 100%;
    text-align: left;
    background: none;
    border: 0;
    padding: 0.45rem 0.75rem;
    font: inherit;
    font-size: 0.875rem;
    cursor: pointer;
    color: #161616;
  }
  .mention-suggest button:hover,
  .mention-suggest button.active {
    background: #e8e8e8;
  }
</style>
