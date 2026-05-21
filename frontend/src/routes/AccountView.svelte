<script lang="ts">
  import { onMount } from "svelte";
  import {
    Button,
    Modal,
    TextInput,
    Checkbox,
    RadioButtonGroup,
    RadioButton,
    Tag,
    InlineNotification,
    DataTable,
    CodeSnippet,
  } from "carbon-components-svelte";
  import Add from "carbon-icons-svelte/lib/Add.svelte";
  import TrashCan from "carbon-icons-svelte/lib/TrashCan.svelte";
  import { listTokens, createToken, revokeToken, type ApiToken } from "../lib/tokens-api";
  import { mfaSetupStart, mfaSetupConfirm, mfaDisable, type MFASetupChallenge } from "../lib/mfa-api";
  import { me, refreshMe } from "../lib/identity";
  import { _ } from "../lib/i18n";

  let tokens: ApiToken[] = [];
  let error = "";

  let createOpen = false;
  let createName = "";
  let createKind: "agent" | "share" = "agent";
  let scopeRead = true;
  let scopeComment = false;
  let scopeWrite = false;
  let sharePath = "";
  let createBusy = false;
  let createError = "";

  let revealOpen = false;
  let revealToken: ApiToken | null = null;

  let verifyBusy = false;
  let verifyResult: { ok: boolean; message: string } | null = null;

  let mfaSetupOpen = false;
  let mfaChallenge: MFASetupChallenge | null = null;
  let mfaConfirmCode = "";
  let mfaConfirmBusy = false;
  let mfaConfirmError = "";

  let mfaDisableOpen = false;
  let mfaDisableCode = "";
  let mfaDisableBusy = false;
  let mfaDisableError = "";

  async function startMFASetup() {
    mfaConfirmError = "";
    mfaConfirmCode = "";
    try {
      mfaChallenge = await mfaSetupStart();
      mfaSetupOpen = true;
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function confirmMFASetup() {
    if (mfaConfirmBusy) return;
    mfaConfirmBusy = true;
    mfaConfirmError = "";
    try {
      await mfaSetupConfirm(mfaConfirmCode.trim());
      await refreshMe();
      mfaSetupOpen = false;
      mfaChallenge = null;
    } catch (err) {
      mfaConfirmError = err instanceof Error ? err.message : "";
    } finally {
      mfaConfirmBusy = false;
    }
  }

  function openDisableMFA() {
    mfaDisableCode = "";
    mfaDisableError = "";
    mfaDisableOpen = true;
  }

  async function confirmDisableMFA() {
    if (mfaDisableBusy) return;
    mfaDisableBusy = true;
    mfaDisableError = "";
    try {
      await mfaDisable(mfaDisableCode.trim());
      await refreshMe();
      mfaDisableOpen = false;
    } catch (err) {
      mfaDisableError = err instanceof Error ? err.message : "";
    } finally {
      mfaDisableBusy = false;
    }
  }

  async function resendVerification() {
    if (verifyBusy) return;
    verifyBusy = true;
    verifyResult = null;
    try {
      const res = await fetch("/api/auth/resend-verification", {
        method: "POST",
        credentials: "same-origin",
      });
      if (res.ok || res.status === 204) {
        verifyResult = { ok: true, message: "" };
      } else {
        let body: any = null;
        try {
          body = await res.json();
        } catch {
          // ignore
        }
        verifyResult = { ok: false, message: body?.error ?? `${res.status}` };
      }
    } catch (err) {
      verifyResult = { ok: false, message: err instanceof Error ? err.message : "" };
    } finally {
      verifyBusy = false;
    }
  }

  onMount(refresh);

  async function refresh() {
    error = "";
    try {
      tokens = await listTokens();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  function openCreate() {
    createName = "";
    createKind = "agent";
    scopeRead = true;
    scopeComment = false;
    scopeWrite = false;
    sharePath = "";
    createError = "";
    createOpen = true;
  }

  function computeScopes(): string[] {
    if (createKind === "share") {
      return sharePath.trim() ? [`read:${sharePath.trim()}`] : [];
    }
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
      createError = $_("account.token_validation");
      return;
    }
    createBusy = true;
    createError = "";
    try {
      const t = await createToken({ name: createName.trim(), scopes });
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

  async function revoke(t: ApiToken) {
    if (!confirm($_("account.token_revoke_confirm", { values: { name: t.name } }))) return;
    try {
      await revokeToken(t.id);
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }
</script>

<section class="account">
  <h1>{$_("account.heading")}</h1>

  {#if $me}
    <p class="dim">
      {$_("account.signed_in_as")}: <strong>{$me.display_name}</strong>
      {#if $me.email}<span class="dim">({$me.email})</span>{/if}
      · {$_("admin.col_role")}: {$me.role}
    </p>

    {#if $me.email}
      <h2>{$_("email_verify.account_heading")}</h2>
      {#if $me.email_verified_at}
        <p class="dim">
          {$_("email_verify.account_verified", { values: { date: new Date($me.email_verified_at).toLocaleString() } })}
        </p>
      {:else}
        <p class="dim">{$_("email_verify.account_not_verified")}</p>
        <div class="actions">
          <Button on:click={resendVerification} disabled={verifyBusy}>
            {verifyBusy ? $_("email_verify.account_resend_busy") : $_("email_verify.account_resend")}
          </Button>
        </div>
        {#if verifyResult}
          <InlineNotification
            kind={verifyResult.ok ? "success" : "error"}
            title={verifyResult.ok ? $_("email_verify.account_resend_ok") : $_("email_verify.account_resend_failed", { values: { error: verifyResult.message } })}
            lowContrast
            hideCloseButton
          />
        {/if}
      {/if}
    {/if}
  {/if}

  {#if $me}
    <h2>{$_("mfa.heading")}</h2>
    {#if $me.totp_enabled_at}
      <p class="dim">{$_("mfa.enabled_since", { values: { date: new Date($me.totp_enabled_at).toLocaleString() } })}</p>
      <div class="actions">
        <Button kind="danger-tertiary" on:click={openDisableMFA}>
          {$_("mfa.disable")}
        </Button>
      </div>
    {:else}
      <p class="dim">{$_("mfa.disabled_hint")}</p>
      <div class="actions">
        <Button on:click={startMFASetup}>{$_("mfa.enable")}</Button>
      </div>
    {/if}
  {/if}

  <h2>{$_("account.tokens_heading")}</h2>
  <p class="dim">{$_("account.tokens_intro")}</p>

  {#if error}
    <InlineNotification kind="error" title={$_("admin.error")} subtitle={error} lowContrast />
  {/if}

  <div class="actions">
    <Button icon={Add} on:click={openCreate}>{$_("account.create_token")}</Button>
  </div>

  <DataTable
    headers={[
      { key: "name", value: $_("account.token_name") },
      { key: "scopes", value: $_("account.token_scopes") },
      { key: "last_used", value: $_("account.token_last_used") },
      { key: "created", value: $_("account.token_created") },
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
          {$_("account.revoke")}
        </Button>
      {:else if cell.key === "last_used" || cell.key === "created"}
        {cell.value === "—" ? "—" : new Date(cell.value).toLocaleString()}
      {:else}
        {cell.value}
      {/if}
    </svelte:fragment>
  </DataTable>
</section>

<Modal
  bind:open={createOpen}
  modalHeading={$_("account.create_token_heading")}
  primaryButtonText={createBusy ? $_("account.creating") : $_("account.create_token")}
  secondaryButtonText={$_("admin.cancel")}
  primaryButtonDisabled={createBusy}
  on:submit={submitCreate}
  on:click:button--secondary={() => (createOpen = false)}
>
  <TextInput
    labelText={$_("account.token_name")}
    placeholder={$_("account.token_name_placeholder")}
    bind:value={createName}
  />

  <div style="margin-top:1rem">
    <RadioButtonGroup bind:selected={createKind} legendText={$_("account.token_kind")}>
      <RadioButton labelText={$_("account.token_kind_agent")} value="agent" />
      <RadioButton labelText={$_("account.token_kind_share")} value="share" />
    </RadioButtonGroup>
  </div>

  {#if createKind === "agent"}
    <div style="margin-top:1rem;display:flex;flex-direction:column;gap:0.4rem">
      <strong style="font-size:0.85rem">{$_("account.token_scopes")}</strong>
      <Checkbox labelText={$_("account.scope_read")} bind:checked={scopeRead} />
      <Checkbox labelText={$_("account.scope_comment")} bind:checked={scopeComment} />
      <Checkbox labelText={$_("account.scope_write")} bind:checked={scopeWrite} />
      <p class="dim" style="margin-top:0.5rem">{$_("account.scope_hint")}</p>
    </div>
  {:else}
    <div style="margin-top:1rem">
      <TextInput
        labelText={$_("account.share_path_label")}
        placeholder={$_("account.share_path_placeholder")}
        bind:value={sharePath}
      />
      <p class="dim" style="margin-top:0.5rem">{$_("account.share_hint")}</p>
    </div>
  {/if}

  {#if createError}
    <div style="margin-top:1rem">
      <InlineNotification kind="error" title={$_("admin.error")} subtitle={createError} lowContrast hideCloseButton />
    </div>
  {/if}
</Modal>

<Modal
  bind:open={revealOpen}
  modalHeading={$_("account.reveal_heading")}
  primaryButtonText={$_("account.reveal_dismiss")}
  on:submit={() => (revealOpen = false)}
  passiveModal={false}
>
  <p style="margin-bottom:1rem">{$_("account.reveal_warning")}</p>
  {#if revealToken?.plaintext}
    <CodeSnippet type="single" code={revealToken.plaintext} />
  {/if}
  <p class="dim" style="margin-top:1rem">{$_("account.reveal_hint")}</p>
</Modal>

<Modal
  bind:open={mfaSetupOpen}
  modalHeading={$_("mfa.setup_heading")}
  primaryButtonText={mfaConfirmBusy ? $_("mfa.verifying") : $_("mfa.confirm")}
  secondaryButtonText={$_("admin.cancel")}
  primaryButtonDisabled={mfaConfirmBusy || mfaConfirmCode.trim().length < 6}
  on:submit={confirmMFASetup}
  on:click:button--secondary={() => (mfaSetupOpen = false)}
>
  <p style="margin-bottom:1rem">{$_("mfa.setup_step1")}</p>
  {#if mfaChallenge}
    <div style="text-align:center;margin-bottom:1rem">
      <img alt="TOTP QR" src={mfaChallenge.qr_png} style="width:220px;height:220px" />
    </div>
    <p class="dim" style="margin-bottom:0.5rem">{$_("mfa.manual_entry")}</p>
    <CodeSnippet type="single" code={mfaChallenge.secret} />
  {/if}
  <p style="margin:1rem 0 0.5rem">{$_("mfa.setup_step2")}</p>
  <TextInput
    labelText={$_("mfa.code_label")}
    placeholder="123456"
    bind:value={mfaConfirmCode}
    maxlength={10}
  />
  {#if mfaConfirmError}
    <div style="margin-top:1rem">
      <InlineNotification kind="error" title={$_("auth.error_title")} subtitle={mfaConfirmError} lowContrast hideCloseButton />
    </div>
  {/if}
</Modal>

<Modal
  bind:open={mfaDisableOpen}
  danger
  modalHeading={$_("mfa.disable_heading")}
  primaryButtonText={mfaDisableBusy ? $_("mfa.disabling") : $_("mfa.disable")}
  secondaryButtonText={$_("admin.cancel")}
  primaryButtonDisabled={mfaDisableBusy || mfaDisableCode.trim().length < 6}
  on:submit={confirmDisableMFA}
  on:click:button--secondary={() => (mfaDisableOpen = false)}
>
  <p style="margin-bottom:1rem">{$_("mfa.disable_prompt")}</p>
  <TextInput
    labelText={$_("mfa.code_label")}
    placeholder="123456"
    bind:value={mfaDisableCode}
    maxlength={10}
  />
  {#if mfaDisableError}
    <div style="margin-top:1rem">
      <InlineNotification kind="error" title={$_("auth.error_title")} subtitle={mfaDisableError} lowContrast hideCloseButton />
    </div>
  {/if}
</Modal>

<style>
  .account {
    max-width: 1440px;
    margin: 0 auto;
    padding: 1rem 1rem 3rem;
  }
  .account h1 {
    font-weight: 600;
    margin: 0.5rem 0 0.75rem;
  }
  .account h2 {
    font-weight: 600;
    margin: 2rem 0 0.5rem;
  }
  .actions {
    margin: 1rem 0;
  }
  .dim {
    color: #6f6a60;
    font-size: 0.9rem;
  }
</style>
