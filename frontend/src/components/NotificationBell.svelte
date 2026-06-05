<script lang="ts">
  import { HeaderAction, HeaderPanelLinks, HeaderPanelDivider, HeaderPanelLink } from "carbon-components-svelte";
  import NotificationIcon from "carbon-icons-svelte/lib/Notification.svelte";
  import { notifications, unreadCount, markRead, markAllRead, refreshNotifications } from "../lib/notifications-store";
  import type { AppNotification } from "../lib/notifications-api";
  import { navigateToDoc } from "../lib/router";
  import { _ } from "../lib/i18n";

  let panelOpen = false;

  // Re-fetch when the panel opens so the list is current even mid-poll-window.
  $: if (panelOpen) void refreshNotifications();

  function text(n: AppNotification): string {
    const key = n.kind === "mention" ? "notifications.mention_text" : "notifications.reply_text";
    return $_(key, { values: { actor: n.actor.display_name, page: n.path } });
  }

  function open(n: AppNotification) {
    void markRead(n.id);
    panelOpen = false;
    navigateToDoc(n.path);
  }
</script>

<div class="bell" class:has-unread={$unreadCount > 0} data-count={$unreadCount > 99 ? "99+" : $unreadCount}>
  <HeaderAction
    bind:isOpen={panelOpen}
    iconDescription={$_("notifications.bell")}
    icon={NotificationIcon}
  >
    <HeaderPanelLinks>
      <HeaderPanelDivider>{$_("notifications.heading")}</HeaderPanelDivider>
      {#if $notifications.length === 0}
        <p class="empty">{$_("notifications.empty")}</p>
      {:else}
        {#if $unreadCount > 0}
          <HeaderPanelLink
            href="#"
            on:click={(e) => {
              e.preventDefault();
              void markAllRead();
            }}
          >
            {$_("notifications.mark_all_read")}
          </HeaderPanelLink>
        {/if}
        {#each $notifications as n (n.id)}
          <HeaderPanelLink
            href={`/docs/${n.path}`}
            class={n.read_at ? "notif read" : "notif"}
            on:click={(e) => {
              e.preventDefault();
              open(n);
            }}
          >
            <span class="dot" class:unread={!n.read_at}></span>
            <span class="text">{text(n)}</span>
            <time class="when">{new Date(n.created_at).toLocaleString()}</time>
          </HeaderPanelLink>
        {/each}
      {/if}
    </HeaderPanelLinks>
  </HeaderAction>
</div>

<style>
  .bell {
    position: relative;
    display: flex;
  }
  /* Unread badge overlaid on the Carbon header button. */
  .bell.has-unread::after {
    content: attr(data-count);
    position: absolute;
    top: 0.35rem;
    right: 0.35rem;
    min-width: 1rem;
    height: 1rem;
    padding: 0 0.2rem;
    border-radius: 999px;
    background: #da1e28;
    color: #ffffff;
    font-size: 0.65rem;
    line-height: 1rem;
    text-align: center;
    pointer-events: none;
    z-index: 1;
  }
  .empty {
    color: #c6c6c6;
    font-size: 0.85rem;
    padding: 0.75rem 1rem;
    margin: 0;
  }
  .bell :global(a.notif) {
    height: auto;
    padding-top: 0.4rem;
    padding-bottom: 0.4rem;
  }
  .bell :global(a.notif .text) {
    display: block;
    white-space: normal;
    line-height: 1.3;
  }
  .bell :global(a.notif.read .text) {
    color: #a8a8a8;
  }
  .bell :global(a.notif .when) {
    display: block;
    font-size: 0.7rem;
    color: #8d8d8d;
    margin-top: 0.15rem;
  }
  .bell :global(a.notif .dot) {
    display: inline-block;
    width: 0.45rem;
    height: 0.45rem;
    border-radius: 50%;
    margin-right: 0.4rem;
    background: transparent;
  }
  .bell :global(a.notif .dot.unread) {
    background: #4589ff;
  }
</style>
