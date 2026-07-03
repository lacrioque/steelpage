<script lang="ts">
  import { onMount } from "svelte";
  import {
    Button,
    Modal,
    TextInput,
    Checkbox,
    Tag,
    InlineNotification,
    DataTable,
    CodeSnippet,
  } from "carbon-components-svelte";
  import Add from "carbon-icons-svelte/lib/Add.svelte";
  import TrashCan from "carbon-icons-svelte/lib/TrashCan.svelte";
  import {
    listMachineTokens,
    createMachineToken,
    revokeMachineToken,
    type MachineToken,
  } from "../lib/machine-tokens-api";
  import { _ } from "../lib/i18n";

  let tokens: MachineToken[] = [];
  let error = "";

  let createOpen = false;
  let createName = "";
  let scopeRead = true;
  let scopeComment = false;
  let scopeWrite = false;
  let createExpiry = "";
  let createBusy = false;
  let createError = "";

  let revealOpen = false;
  let revealToken: MachineToken | null = null;

  onMount(refresh);

  async function refresh() {
    error = "";
    try {
      tokens = await listMachineTokens();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  function openCreate() {
    createName = "";
    scopeRead = true;
    scopeComment = false;
    scopeWrite = false;
    createExpiry = "";
    createError = "";
    createOpen = true;
  }

  function computeScopes(): string[] {
    const out: string[] = [];
    if (scopeRead) out.push("read");
    if (scopeComment) out.push("comment");
    if (scopeWrite) out.push("write");
    return out;
  }

  async function submitCreate() {
    if (createBusy) return;
    const scopes = computeScopes();
    if (!createName.trim() || scopes.length === 0) {
      createError = $_("admin.machine_validation");
      return;
    }
    createBusy = true;
    createError = "";
    try {
      const t = await createMachineToken({
        name: createName.trim(),
        scopes,
        expires_at: createExpiry.trim() || undefined,
      });
      createOpen = false;
      revealToken = t;
      revealOpen = true;
      await refresh();
    } catch (err) {
      createError = err instanceof Error ? err.message : "";
    } finally {
      createBusy = false;
    }
  }

  async function revoke(t: MachineToken) {
    if (!confirm($_("admin.machine_revoke_confirm", { values: { name: t.name } }))) return;
    try {
      await revokeMachineToken(t.id);
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  function mcpConfigSnippet(plaintext: string): string {
    const origin = typeof window !== "undefined" ? window.location.origin : "https://wiki.example.com";
    return JSON.stringify(
      {
        mcpServers: {
          steelpage: {
            type: "http",
            url: `${origin}/mcp`,
            headers: {
              Authorization: `Bearer ${plaintext}`,
            },
          },
        },
      },
      null,
      2,
    );
  }
</script>

<div class="machine-tokens">
  <p class="dim">{$_("admin.machine_intro")}</p>

  {#if error}
    <InlineNotification kind="error" title={$_("admin.error")} subtitle={error} lowContrast />
  {/if}

  <div class="actions">
    <Button icon={Add} on:click={openCreate}>{$_("admin.machine_create")}</Button>
  </div>

  <DataTable
    headers={[
      { key: "name", value: $_("admin.machine_name") },
      { key: "scopes", value: $_("admin.machine_scopes") },
      { key: "last_used", value: $_("admin.machine_last_used") },
      { key: "created", value: $_("admin.machine_created") },
      { key: "actions", value: "" },
    ]}
    rows={tokens.map((t) => ({
      id: String(t.id),
      name: t.name,
      scopes: t.scopes,
      last_used: t.last_used_at ?? "—",
      created: t.created_at,
      _token: t,
    }))}
  >
    <svelte:fragment slot="cell" let:row let:cell>
      {#if cell.key === "scopes"}
        {#each cell.value as s (s)}
          <Tag type="cool-gray" size="sm">{s}</Tag>
        {/each}
      {:else if cell.key === "actions"}
        <Button kind="danger-ghost" size="sm" icon={TrashCan} on:click={() => revoke((row as any)._token)}>
          {$_("admin.machine_revoke")}
        </Button>
      {:else if cell.key === "last_used" || cell.key === "created"}
        {cell.value === "—" ? "—" : new Date(cell.value).toLocaleString()}
      {:else}
        {cell.value}
      {/if}
    </svelte:fragment>
  </DataTable>
</div>

<Modal
  bind:open={createOpen}
  modalHeading={$_("admin.machine_create_heading")}
  primaryButtonText={createBusy ? $_("admin.machine_creating") : $_("admin.machine_create")}
  secondaryButtonText={$_("admin.cancel")}
  primaryButtonDisabled={createBusy}
  on:submit={submitCreate}
  on:click:button--secondary={() => (createOpen = false)}
>
  <TextInput
    labelText={$_("admin.machine_name")}
    placeholder={$_("admin.machine_name_placeholder")}
    bind:value={createName}
  />

  <div style="margin-top:1rem;display:flex;flex-direction:column;gap:0.4rem">
    <strong style="font-size:0.85rem">{$_("admin.machine_scopes")}</strong>
    <Checkbox labelText={$_("admin.machine_scope_read")} bind:checked={scopeRead} />
    <Checkbox labelText={$_("admin.machine_scope_comment")} bind:checked={scopeComment} />
    <Checkbox labelText={$_("admin.machine_scope_write")} bind:checked={scopeWrite} />
    <p class="dim" style="margin-top:0.5rem">{$_("admin.machine_scope_hint")}</p>
  </div>

  <div style="margin-top:1rem">
    <TextInput
      labelText={$_("admin.machine_expiry")}
      placeholder={$_("admin.machine_expiry_placeholder")}
      bind:value={createExpiry}
    />
    <p class="dim" style="margin-top:0.5rem">{$_("admin.machine_expiry_hint")}</p>
  </div>

  {#if createError}
    <div style="margin-top:1rem">
      <InlineNotification kind="error" title={$_("admin.error")} subtitle={createError} lowContrast hideCloseButton />
    </div>
  {/if}
</Modal>

<Modal
  bind:open={revealOpen}
  modalHeading={$_("admin.machine_reveal_heading")}
  primaryButtonText={$_("admin.machine_reveal_dismiss")}
  on:submit={() => (revealOpen = false)}
  passiveModal={false}
>
  <p style="margin-bottom:1rem">{$_("admin.machine_reveal_warning")}</p>
  {#if revealToken?.plaintext}
    <CodeSnippet type="single" code={revealToken.plaintext} />
    <h4 style="margin:1.5rem 0 0.5rem;font-size:0.95rem;font-weight:600">
      {$_("admin.machine_mcp_heading")}
    </h4>
    <p class="dim" style="margin-bottom:0.5rem">{$_("admin.machine_mcp_hint")}</p>
    <CodeSnippet type="multi" code={mcpConfigSnippet(revealToken.plaintext)} />
  {/if}
  <p class="dim" style="margin-top:1rem">{$_("admin.machine_reveal_hint")}</p>
</Modal>

<style>
  .machine-tokens .actions {
    margin: 1rem 0;
  }
  .dim {
    color: #6f6a60;
    font-size: 0.9rem;
  }
</style>
