<script lang="ts">
  import { onMount } from "svelte";
  import {
    Header,
    HeaderUtilities,
    HeaderGlobalAction,
    SideNav,
    Content,
    SkipToContent,
    InlineLoading,
    Breadcrumb,
    BreadcrumbItem,
  } from "carbon-components-svelte";
  import Edit from "carbon-icons-svelte/lib/Edit.svelte";
  import View from "carbon-icons-svelte/lib/View.svelte";
  import Save from "carbon-icons-svelte/lib/Save.svelte";
  import Printer from "carbon-icons-svelte/lib/Printer.svelte";
  import Bot from "carbon-icons-svelte/lib/Bot.svelte";
  import ChatLaunch from "carbon-icons-svelte/lib/ChatLaunch.svelte";
  import UserAvatar from "carbon-icons-svelte/lib/UserAvatar.svelte";
  import AddComment from "carbon-icons-svelte/lib/AddComment.svelte";
  import SearchIcon from "carbon-icons-svelte/lib/Search.svelte";
  import LogoutIcon from "carbon-icons-svelte/lib/Logout.svelte";
  import Settings from "carbon-icons-svelte/lib/Settings.svelte";

  import ArchiveTree from "../components/ArchiveTree.svelte";
  import DocumentView from "./DocumentView.svelte";
  import LoginView from "./LoginView.svelte";
  import AdminView from "./AdminView.svelte";
  import AccountView from "./AccountView.svelte";
  import ForgotView from "./ForgotView.svelte";
  import ResetView from "./ResetView.svelte";
  import VerifyView from "./VerifyView.svelte";
  import EmailVerifyBanner from "../components/EmailVerifyBanner.svelte";
  import CommentsSidebar from "../components/CommentsSidebar.svelte";
  import AddCommentModal from "../components/AddCommentModal.svelte";
  import LocaleToggle from "../components/LocaleToggle.svelte";
  import SearchOverlay from "../components/SearchOverlay.svelte";
  import VersionMenu from "../components/VersionMenu.svelte";
  import { currentDoc, navigateToDoc, navigateToLogin, navigateToAdmin, navigateToAccount, routeKind } from "../lib/router";
  import { me, refreshMe, logout } from "../lib/identity";
  import { getCapabilities, type AuthCapabilities } from "../lib/auth-api";
  import {
    doc as docStore,
    editing,
    saveState,
    saveError,
    toggleEdit,
    save,
  } from "../lib/document-store";
  import {
    getCurrentLine,
    focusLine,
    setOnMarkerClick,
    setOnEmptyGutterClick,
  } from "../lib/editor";
  import { _ } from "../lib/i18n";

  let isSideNavOpen = false;
  let showComments = false;
  let addCommentOpen = false;
  let addCommentLine = 1;
  let addCommentAnchor = "";
  let addCommentReplyTo: number | null = null;
  let addCommentReplyAuthor = "";
  let searchOpen = false;
  let caps: AuthCapabilities | null = null;

  // Replies start from the parent comment's anchor — same line + same
  // captured text — so they re-anchor along with the conversation.
  function startReply(event: CustomEvent<{ parent: import("../lib/types").Comment }>) {
    if (!$me) {
      navigateToLogin();
      return;
    }
    const p = event.detail.parent;
    addCommentLine = p.line_start;
    addCommentAnchor = p.anchor_text;
    addCommentReplyTo = p.id;
    addCommentReplyAuthor = p.author.display_name;
    addCommentOpen = true;
  }

  function onKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      searchOpen = true;
    }
  }

  onMount(async () => {
    await refreshMe();
    try {
      caps = await getCapabilities();
    } catch {
      caps = { local_enabled: true, allow_anonymous_read: true, providers: [] };
    }
    window.addEventListener("keydown", onKeydown);

    setOnMarkerClick((line: number) => {
      showComments = true;
      focusLine(line);
    });

    setOnEmptyGutterClick((line: number, text: string) => {
      if (!$me) {
        navigateToLogin();
        return;
      }
      addCommentLine = line;
      addCommentAnchor = text;
      addCommentReplyTo = null;
      addCommentReplyAuthor = "";
      addCommentOpen = true;
    });

    return () => {
      window.removeEventListener("keydown", onKeydown);
      setOnMarkerClick(null);
      setOnEmptyGutterClick(null);
    };
  });

  // Route guards.
  //
  // 1. /admin and /account always require a session — don't even render the
  //    view to anonymous visitors. Showing a half-functional page that 401s
  //    every API call is a worse UX than a clean bounce to /login.
  // 2. /admin additionally requires role=admin. Authenticated non-admins
  //    land on the doc root, which is where they actually have access.
  // 3. Doc reads: only push to /login when the server actually requires
  //    auth to read. /forgot, /reset and /verify stay reachable without
  //    a session — operators who can't sign in are exactly the ones who
  //    need them.
  $: if (caps && $me === null && ($routeKind === "admin" || $routeKind === "account")) {
    navigateToLogin();
  } else if (caps && $me && $routeKind === "admin" && $me.role !== "admin") {
    navigateToDoc("README.md");
  } else if (
    caps &&
    !caps.allow_anonymous_read &&
    $me === null &&
    $routeKind !== "login" &&
    $routeKind !== "forgot" &&
    $routeKind !== "reset" &&
    $routeKind !== "verify"
  ) {
    navigateToLogin();
  }

  function openAddComment() {
    if (!$me) {
      navigateToLogin();
      return;
    }
    const current = getCurrentLine();
    if (current) {
      addCommentLine = current.number;
      addCommentAnchor = current.text;
    } else {
      addCommentLine = 1;
      addCommentAnchor = "";
    }
    addCommentReplyTo = null;
    addCommentReplyAuthor = "";
    addCommentOpen = true;
  }

  async function handleLogout() {
    await logout();
    navigateToLogin();
  }

  function segments(path: string): { name: string; path: string }[] {
    const parts = path.split("/").filter(Boolean);
    return parts.map((name, idx) => ({
      name,
      path: parts.slice(0, idx + 1).join("/"),
    }));
  }
</script>

<Header platformName={$_("shell.platform_name")} href="/" bind:isSideNavOpen>
  <svelte:fragment slot="skip-to-content">
    <SkipToContent />
  </svelte:fragment>

  <HeaderUtilities>
    {#if $routeKind === "doc" && $docStore}
      {#if $me && $editing && !$docStore.viewing_ref}
        <HeaderGlobalAction
          iconDescription={$_("shell.add_comment_current_line")}
          icon={AddComment}
          on:click={openAddComment}
        />
      {/if}

      {#if $me && !$docStore.viewing_ref}
        <HeaderGlobalAction
          iconDescription={$editing ? $_("shell.switch_to_read") : $_("shell.switch_to_edit")}
          icon={$editing ? View : Edit}
          on:click={toggleEdit}
        />
      {/if}

      {#if $me && $editing && !$docStore.viewing_ref}
        <HeaderGlobalAction
          iconDescription={$_("shell.save")}
          icon={Save}
          on:click={save}
          isActive={$saveState === "saving"}
        />
      {/if}

      <HeaderGlobalAction
        iconDescription={$_("shell.print")}
        icon={Printer}
        on:click={() => window.print()}
      />

      <HeaderGlobalAction
        iconDescription={$_("shell.toggle_comments")}
        icon={ChatLaunch}
        isActive={showComments}
        on:click={() => (showComments = !showComments)}
      />

      <HeaderGlobalAction
        iconDescription={$_("shell.bot_ready")}
        icon={Bot}
        on:click={() => window.open(`/docs/${$docStore.path}?botready=1`, "_blank")}
      />
    {/if}

    {#if $routeKind === "doc"}
      <HeaderGlobalAction
        iconDescription={$_("search.open")}
        icon={SearchIcon}
        on:click={() => (searchOpen = true)}
      />
    {/if}

    <LocaleToggle />

    {#if $me?.role === "admin"}
      <HeaderGlobalAction
        iconDescription={$_("shell.admin")}
        icon={Settings}
        isActive={$routeKind === "admin"}
        on:click={navigateToAdmin}
      />
    {/if}

    {#if $me}
      <HeaderGlobalAction
        iconDescription={$_("shell.signed_in_as", { values: { name: $me.display_name } })}
        icon={UserAvatar}
        on:click={navigateToAccount}
      />
      <HeaderGlobalAction
        iconDescription={$_("shell.sign_out")}
        icon={LogoutIcon}
        on:click={handleLogout}
      />
    {:else}
      <HeaderGlobalAction
        iconDescription={$_("shell.sign_in")}
        icon={UserAvatar}
        on:click={navigateToLogin}
      />
    {/if}
  </HeaderUtilities>
</Header>

{#if $routeKind === "doc"}
  <SideNav bind:isOpen={isSideNavOpen} aria-label={$_("shell.archive_nav_label")}>
    <ArchiveTree />
  </SideNav>
{/if}

<Content id="main-content">
  {#if $me && !$me.email_verified_at && $me.email && $routeKind === "doc"}
    <EmailVerifyBanner />
  {/if}
  {#if $routeKind === "login"}
    <LoginView />
  {:else if $routeKind === "forgot"}
    <ForgotView />
  {:else if $routeKind === "reset"}
    <ResetView />
  {:else if $routeKind === "verify"}
    <VerifyView />
  {:else if $routeKind === "admin"}
    <AdminView />
  {:else if $routeKind === "account"}
    <AccountView />
  {:else}
    <div class="bread">
      <Breadcrumb noTrailingSlash>
        <BreadcrumbItem
          href="/docs/README.md"
          on:click={(e) => {
            e.preventDefault();
            navigateToDoc("README.md");
          }}
        >
          {$_("shell.breadcrumb_root")}
        </BreadcrumbItem>
        {#each segments($currentDoc) as seg, i (seg.path)}
          <BreadcrumbItem
            href={`/docs/${seg.path}`}
            isCurrentPage={i === segments($currentDoc).length - 1}
            on:click={(e) => {
              e.preventDefault();
              navigateToDoc(seg.path);
            }}
          >
            {seg.name}
          </BreadcrumbItem>
        {/each}
      </Breadcrumb>

      <div style="flex:1"></div>

      {#if $saveState === "saving"}
        <InlineLoading status="active" description={$_("shell.save_status_saving")} />
      {:else if $saveState === "saved"}
        <InlineLoading status="finished" description={$_("shell.save_status_saved")} />
      {:else if $saveState === "error"}
        <InlineLoading status="error" description={$saveError || $_("shell.save_status_failed")} />
      {/if}

      {#if $docStore}
        <VersionMenu
          path={$docStore.path}
          version={($docStore.frontmatter.version ?? "?") as string | number}
          sha={$docStore.sha}
          viewingRef={$docStore.viewing_ref}
        />
      {/if}
    </div>

    <div class="layout" class:with-comments={showComments && $docStore}>
      <section class="doc">
        <DocumentView />
      </section>
      {#if showComments && $docStore}
        <CommentsSidebar on:reply={startReply} />
      {/if}
    </div>
  {/if}
</Content>

{#if $docStore}
  <AddCommentModal
    bind:open={addCommentOpen}
    path={$docStore.path}
    lineNumber={addCommentLine}
    anchorText={addCommentAnchor}
    replyTo={addCommentReplyTo}
    replyToAuthor={addCommentReplyAuthor}
  />
{/if}

<SearchOverlay bind:open={searchOpen} />

<style>
  .bread {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.5rem 0 1rem;
    flex-wrap: wrap;
  }
  .dim {
    color: #6f6a60;
    font-size: 0.8rem;
  }
  .layout {
    display: grid;
    grid-template-columns: 1fr;
    gap: 0;
    min-height: calc(100vh - 200px);
  }
  .layout.with-comments {
    grid-template-columns: 1fr 320px;
  }
  .doc {
    min-width: 0;
  }
  @media (max-width: 1000px) {
    .layout.with-comments {
      grid-template-columns: 1fr;
    }
  }
</style>
