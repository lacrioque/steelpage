<script lang="ts">
  import { onMount } from "svelte";
  import { Tile, Button, InlineLoading, InlineNotification } from "carbon-components-svelte";
  import { navigate } from "../lib/router";
  import { refreshMe } from "../lib/identity";
  import { _ } from "../lib/i18n";

  type Phase = "verifying" | "ok" | "error" | "missing";
  let phase: Phase = "verifying";
  let message = "";

  onMount(async () => {
    const token = new URLSearchParams(window.location.search).get("token");
    if (!token) {
      phase = "missing";
      return;
    }
    try {
      const res = await fetch("/api/auth/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ token }),
      });
      if (res.ok || res.status === 204) {
        phase = "ok";
        await refreshMe();
      } else {
        let body: any = null;
        try {
          body = await res.json();
        } catch {
          // ignore
        }
        message = body?.error ?? `Verify failed (${res.status})`;
        phase = "error";
      }
    } catch (err) {
      message = err instanceof Error ? err.message : "Network error";
      phase = "error";
    }
  });
</script>

<section class="verify-page">
  <Tile light>
    <h1>{$_("email_verify.heading")}</h1>

    {#if phase === "verifying"}
      <InlineLoading status="active" description={$_("email_verify.verifying")} />
    {:else if phase === "ok"}
      <InlineNotification
        kind="success"
        title={$_("email_verify.ok_title")}
        subtitle={$_("email_verify.ok_hint")}
        lowContrast
        hideCloseButton
      />
      <div class="actions">
        <Button on:click={() => navigate("/")}>{$_("email_verify.continue")}</Button>
      </div>
    {:else if phase === "missing"}
      <InlineNotification
        kind="warning"
        title={$_("email_verify.missing_title")}
        subtitle={$_("email_verify.missing_hint")}
        lowContrast
        hideCloseButton
      />
    {:else}
      <InlineNotification
        kind="error"
        title={$_("email_verify.error_title")}
        subtitle={message || $_("email_verify.error_hint")}
        lowContrast
        hideCloseButton
      />
      <div class="actions">
        <Button kind="tertiary" on:click={() => navigate("/account")}>
          {$_("email_verify.go_to_account")}
        </Button>
      </div>
    {/if}
  </Tile>
</section>

<style>
  .verify-page {
    max-width: 480px;
    margin: 4rem auto;
    padding: 0 1.25rem;
  }
  .verify-page h1 {
    margin: 0 0 1rem;
    font-size: 1.4rem;
    font-weight: 600;
  }
  .actions {
    margin-top: 1.5rem;
  }
</style>
