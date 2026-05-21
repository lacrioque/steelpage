<script lang="ts">
  import { onMount } from "svelte";
  import {
    PasswordInput,
    Button,
    Tile,
    InlineNotification,
  } from "carbon-components-svelte";
  import { navigate } from "../lib/router";
  import { _ } from "../lib/i18n";

  let token = "";
  let newPassword = "";
  let confirmPassword = "";
  let busy = false;
  let done = false;
  let error = "";

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    token = params.get("token") ?? "";
  });

  $: clientError = (() => {
    if (!newPassword && !confirmPassword) return "";
    if (newPassword.length < 8) return $_("password_reset.password_too_short");
    if (newPassword !== confirmPassword) return $_("password_reset.password_mismatch");
    return "";
  })();

  async function submit() {
    if (busy) return;
    error = "";
    if (newPassword.length < 8) {
      error = $_("password_reset.password_too_short");
      return;
    }
    if (newPassword !== confirmPassword) {
      error = $_("password_reset.password_mismatch");
      return;
    }
    busy = true;
    try {
      const res = await fetch("/api/auth/reset", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ token, new_password: newPassword }),
      });
      if (!res.ok && res.status !== 204) {
        let msg = `Reset failed (${res.status})`;
        try {
          const body = await res.json();
          if (body && typeof body.error === "string") msg = body.error;
        } catch {
          // ignore
        }
        error = msg;
        return;
      }
      done = true;
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      busy = false;
    }
  }
</script>

<section class="reset-page">
  <Tile light>
    <h1>{$_("password_reset.reset_heading")}</h1>

    {#if !token}
      <InlineNotification
        kind="warning"
        title={$_("password_reset.error_title")}
        subtitle={$_("password_reset.reset_intro_no_token")}
        lowContrast
        hideCloseButton
      />
      <div class="actions">
        <Button
          kind="tertiary"
          on:click={(e) => {
            e.preventDefault();
            navigate("/login");
          }}
        >
          {$_("password_reset.back_to_login")}
        </Button>
      </div>
    {:else if done}
      <InlineNotification
        kind="success"
        title={$_("password_reset.reset_done_title")}
        subtitle={$_("password_reset.reset_done_hint")}
        lowContrast
        hideCloseButton
      />
      <div class="actions">
        <Button
          on:click={(e) => {
            e.preventDefault();
            navigate("/login");
          }}
        >
          {$_("password_reset.back_to_login")}
        </Button>
      </div>
    {:else}
      <p class="hint">{$_("password_reset.reset_intro")}</p>
      <form on:submit|preventDefault={submit} class="form">
        <PasswordInput
          labelText={$_("password_reset.new_password")}
          bind:value={newPassword}
          required
        />
        <PasswordInput
          labelText={$_("password_reset.confirm_password")}
          bind:value={confirmPassword}
          required
        />
        {#if clientError}
          <InlineNotification
            kind="warning"
            title={$_("password_reset.error_title")}
            subtitle={clientError}
            lowContrast
            hideCloseButton
          />
        {/if}
        {#if error}
          <InlineNotification
            kind="error"
            title={$_("password_reset.error_title")}
            subtitle={error}
            lowContrast
            hideCloseButton
          />
        {/if}
        <Button
          type="submit"
          disabled={busy ||
            newPassword.length < 8 ||
            newPassword !== confirmPassword}
        >
          {busy ? $_("password_reset.reset_busy") : $_("password_reset.reset_submit")}
        </Button>
      </form>
    {/if}
  </Tile>
</section>

<style>
  .reset-page {
    max-width: 480px;
    margin: 4rem auto;
    padding: 0 1.25rem;
  }
  .reset-page h1 {
    margin: 0 0 0.5rem;
    font-weight: 600;
    font-size: 1.5rem;
  }
  .hint {
    color: #525252;
    margin-bottom: 1.5rem;
  }
  .form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding-top: 1rem;
  }
  .actions {
    margin-top: 1.5rem;
  }
</style>
