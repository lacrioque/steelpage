<script lang="ts">
  import { InlineNotification, Button } from "carbon-components-svelte";
  import { _ } from "../lib/i18n";

  let busy = false;
  let result: { ok: boolean; message: string } | null = null;

  async function resend() {
    if (busy) return;
    busy = true;
    result = null;
    try {
      const res = await fetch("/api/auth/resend-verification", {
        method: "POST",
        credentials: "same-origin",
      });
      if (res.ok || res.status === 204) {
        result = { ok: true, message: $_("email_verify.resend_sent") };
      } else {
        let body: any = null;
        try {
          body = await res.json();
        } catch {
          // ignore
        }
        result = { ok: false, message: body?.error ?? `${res.status}` };
      }
    } catch (err) {
      result = { ok: false, message: err instanceof Error ? err.message : "" };
    } finally {
      busy = false;
    }
  }
</script>

<div class="banner">
  <InlineNotification
    kind={result?.ok ? "success" : "warning"}
    title={result?.ok ? $_("email_verify.banner_sent_title") : $_("email_verify.banner_title")}
    subtitle={result?.ok ? result.message : $_("email_verify.banner_hint")}
    lowContrast
    hideCloseButton
  >
    <svelte:fragment slot="actions">
      {#if !result?.ok}
        <Button kind="ghost" size="sm" on:click={resend} disabled={busy}>
          {busy ? $_("email_verify.resend_busy") : $_("email_verify.resend")}
        </Button>
      {/if}
    </svelte:fragment>
  </InlineNotification>
</div>

<style>
  .banner {
    padding: 0 1rem;
    margin-top: -0.5rem;
  }
</style>
