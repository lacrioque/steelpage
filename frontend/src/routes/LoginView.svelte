<script lang="ts">
  import { onMount } from "svelte";
  import {
    Tabs,
    Tab,
    TabContent,
    TextInput,
    PasswordInput,
    Button,
    InlineNotification,
    Tile,
  } from "carbon-components-svelte";
  import Login from "carbon-icons-svelte/lib/Login.svelte";
  import LogoOkta from "carbon-icons-svelte/lib/Wikis.svelte";
  import {
    getCapabilities,
    login as apiLogin,
    register as apiRegister,
    type AuthCapabilities,
  } from "../lib/auth-api";
  import { mfaLogin } from "../lib/mfa-api";
  import { setMe } from "../lib/identity";
  import { navigate, navigateToDoc } from "../lib/router";
  import { _ } from "../lib/i18n";

  let caps: AuthCapabilities | null = null;
  let active = 0;

  // ?oidc_error=mismatch is set by the backend when an OIDC callback would
  // collide with an already-linked email — show a clear banner so the user
  // knows why they weren't signed in.
  const oidcError = new URLSearchParams(window.location.search).get("oidc_error");

  let loginEmail = "";
  let loginPassword = "";
  let loginBusy = false;
  let loginError = "";

  let regEmail = "";
  let regPassword = "";
  let regDisplayName = "";
  let regBusy = false;
  let regError = "";

  // MFA: when login returns {mfa_required:true} we hide the email/password
  // tabs and show a code-only step. The session cookie is half-authenticated
  // server-side at that point.
  let mfaPending = false;
  let mfaCode = "";
  let mfaBusy = false;
  let mfaError = "";

  onMount(async () => {
    try {
      caps = await getCapabilities();
    } catch (err) {
      loginError = err instanceof Error ? err.message : "";
    }
  });

  async function submitLogin() {
    if (loginBusy) return;
    loginBusy = true;
    loginError = "";
    try {
      const result = await apiLogin(loginEmail, loginPassword);
      if (result.kind === "mfa_required") {
        mfaPending = true;
        mfaCode = "";
        mfaError = "";
        return;
      }
      setMe(result.user);
      navigateToDoc("README.md");
    } catch (err) {
      loginError = err instanceof Error ? err.message : "";
    } finally {
      loginBusy = false;
    }
  }

  async function submitMFA() {
    if (mfaBusy) return;
    mfaBusy = true;
    mfaError = "";
    try {
      const u = await mfaLogin(mfaCode.trim());
      setMe(u);
      navigateToDoc("README.md");
    } catch (err) {
      mfaError = err instanceof Error ? err.message : "";
    } finally {
      mfaBusy = false;
    }
  }

  function cancelMFA() {
    mfaPending = false;
    mfaCode = "";
    mfaError = "";
    loginPassword = "";
  }

  async function submitRegister() {
    if (regBusy) return;
    regBusy = true;
    regError = "";
    try {
      const m = await apiRegister({
        email: regEmail,
        password: regPassword,
        display_name: regDisplayName,
      });
      setMe(m);
      navigateToDoc("README.md");
    } catch (err) {
      regError = err instanceof Error ? err.message : "";
    } finally {
      regBusy = false;
    }
  }
</script>

<section class="login-page">
  <Tile light>
    <div class="brand">
      <img src="/logo.svg" alt="Steelpage" width="56" height="56" />
      <h1>{$_("auth.heading")}</h1>
    </div>
    <p class="hint">{$_("auth.intro")}</p>

    {#if oidcError === "mismatch"}
      <InlineNotification
        kind="error"
        title={$_("auth.oidc_mismatch_title")}
        subtitle={$_("auth.oidc_mismatch_hint")}
        lowContrast
        hideCloseButton
      />
    {/if}

    {#if mfaPending}
      <form on:submit|preventDefault={submitMFA} class="form">
        <p class="hint">{$_("mfa.login_prompt")}</p>
        <TextInput
          labelText={$_("mfa.code_label")}
          placeholder="123456"
          bind:value={mfaCode}
          autocomplete="one-time-code"
          maxlength={10}
          required
        />
        {#if mfaError}
          <InlineNotification
            kind="error"
            title={$_("auth.error_title")}
            subtitle={mfaError}
            lowContrast
            hideCloseButton
          />
        {/if}
        <Button type="submit" disabled={mfaBusy || mfaCode.trim().length < 6}>
          {mfaBusy ? $_("mfa.verifying") : $_("mfa.verify")}
        </Button>
        <Button kind="ghost" on:click={cancelMFA}>
          {$_("mfa.cancel")}
        </Button>
      </form>
    {:else if caps?.local_enabled}
      <Tabs bind:selected={active}>
        <Tab label={$_("auth.tab_sign_in")} />
        <Tab label={$_("auth.tab_create")} />
        <svelte:fragment slot="content">
          <TabContent>
            <form on:submit|preventDefault={submitLogin} class="form">
              <TextInput
                labelText={$_("auth.email")}
                placeholder={$_("auth.email_placeholder")}
                bind:value={loginEmail}
                type="email"
                required
              />
              <PasswordInput
                labelText={$_("auth.password")}
                bind:value={loginPassword}
                required
              />
              {#if loginError}
                <InlineNotification
                  kind="error"
                  title={$_("auth.error_title")}
                  subtitle={loginError}
                  lowContrast
                  hideCloseButton
                />
              {/if}
              <Button
                type="submit"
                icon={Login}
                disabled={loginBusy || !loginEmail || !loginPassword}
              >
                {loginBusy ? $_("auth.signing_in") : $_("auth.sign_in")}
              </Button>
              <p class="forgot">
                <a
                  href="/forgot"
                  on:click={(e) => {
                    e.preventDefault();
                    navigate("/forgot");
                  }}
                >
                  {$_("password_reset.forgot_link")}
                </a>
              </p>
            </form>
          </TabContent>
          <TabContent>
            <form on:submit|preventDefault={submitRegister} class="form">
              <TextInput
                labelText={$_("auth.display_name")}
                placeholder={$_("auth.display_name_placeholder")}
                bind:value={regDisplayName}
                maxlength={64}
                required
              />
              <TextInput
                labelText={$_("auth.email")}
                placeholder={$_("auth.email_placeholder")}
                bind:value={regEmail}
                type="email"
                required
              />
              <PasswordInput
                labelText={$_("auth.password")}
                helperText={$_("auth.password_help")}
                bind:value={regPassword}
                required
              />
              {#if regError}
                <InlineNotification
                  kind="error"
                  title={$_("auth.error_title")}
                  subtitle={regError}
                  lowContrast
                  hideCloseButton
                />
              {/if}
              <Button
                type="submit"
                disabled={regBusy || !regEmail || regPassword.length < 8 || !regDisplayName}
              >
                {regBusy ? $_("auth.creating") : $_("auth.create_account")}
              </Button>
            </form>
          </TabContent>
        </svelte:fragment>
      </Tabs>
    {:else}
      <p>{$_("auth.local_disabled")}</p>
    {/if}

    {#if caps && caps.providers.length > 0}
      <div class="providers">
        <p class="dim">{$_("auth.or_continue_with")}</p>
        {#each caps.providers as p (p.name)}
          <a href="/api/auth/oidc/start" class="provider-link">
            <Button kind="tertiary" icon={LogoOkta}>{p.label}</Button>
          </a>
        {/each}
      </div>
    {/if}
  </Tile>
</section>

<style>
  .login-page {
    max-width: 480px;
    margin: 4rem auto;
    padding: 0 1.25rem;
  }
  .login-page h1 {
    margin: 0 0 0.5rem;
    font-weight: 600;
    font-size: 1.5rem;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 0.85rem;
    margin-bottom: 0.5rem;
  }
  .brand img {
    display: block;
  }
  .brand h1 {
    margin: 0;
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
  .providers {
    margin-top: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    align-items: flex-start;
  }
  .dim {
    color: #6f6a60;
    font-size: 0.85rem;
  }
  .provider-link {
    text-decoration: none;
  }
  .forgot {
    margin: 0;
    font-size: 0.85rem;
  }
  .forgot a {
    color: #0f62fe;
    text-decoration: none;
  }
  .forgot a:hover {
    text-decoration: underline;
  }
</style>
