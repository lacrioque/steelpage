<script lang="ts">
  import {
    TextInput,
    Button,
    Tile,
    InlineNotification,
  } from "carbon-components-svelte";
  import { navigate } from "../lib/router";
  import { _ } from "../lib/i18n";

  let email = "";
  let busy = false;
  let sent = false;

  async function submit() {
    if (busy) return;
    busy = true;
    try {
      // Fire-and-forget: the API answers 204 regardless of whether the email
      // exists. We always show the same confirmation to avoid leaking which
      // addresses are registered.
      await fetch("/api/auth/forgot", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ email }),
      });
    } catch {
      // Network errors are also swallowed for the same reason.
    } finally {
      busy = false;
      sent = true;
    }
  }
</script>

<section class="forgot-page">
  <Tile light>
    <h1>{$_("password_reset.forgot_heading")}</h1>
    <p class="hint">{$_("password_reset.forgot_intro")}</p>

    {#if sent}
      <InlineNotification
        kind="success"
        title={$_("password_reset.forgot_sent_title")}
        subtitle={$_("password_reset.forgot_sent_hint")}
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
    {:else}
      <form on:submit|preventDefault={submit} class="form">
        <TextInput
          labelText={$_("password_reset.forgot_email_label")}
          placeholder="you@example.com"
          bind:value={email}
          type="email"
          required
        />
        <Button type="submit" disabled={busy || !email}>
          {busy ? $_("password_reset.forgot_busy") : $_("password_reset.forgot_submit")}
        </Button>
        <p class="back">
          <a
            href="/login"
            on:click={(e) => {
              e.preventDefault();
              navigate("/login");
            }}
          >
            {$_("password_reset.back_to_login")}
          </a>
        </p>
      </form>
    {/if}
  </Tile>
</section>

<style>
  .forgot-page {
    max-width: 480px;
    margin: 4rem auto;
    padding: 0 1.25rem;
  }
  .forgot-page h1 {
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
  .back {
    margin: 0;
    font-size: 0.85rem;
  }
  .back a {
    color: #0f62fe;
    text-decoration: none;
  }
  .back a:hover {
    text-decoration: underline;
  }
</style>
