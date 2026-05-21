<script lang="ts">
  import { HeaderAction, HeaderPanelLinks, HeaderPanelDivider, HeaderPanelLink } from "carbon-components-svelte";
  import Language from "carbon-icons-svelte/lib/Language.svelte";
  import { _, locale, setLocale, SUPPORTED_LOCALES, type SupportedLocale } from "../lib/i18n";

  const labels: Record<SupportedLocale, string> = {
    en: "English",
    de: "Deutsch",
  };

  let panelOpen = false;

  function pick(code: SupportedLocale) {
    setLocale(code);
    panelOpen = false;
  }
</script>

<HeaderAction
  bind:isOpen={panelOpen}
  iconDescription={$_("shell.locale_toggle")}
  icon={Language}
>
  <HeaderPanelLinks>
    <HeaderPanelDivider>{$_("shell.locale_toggle")}</HeaderPanelDivider>
    {#each SUPPORTED_LOCALES as code (code)}
      <HeaderPanelLink
        href="#"
        on:click={(e) => {
          e.preventDefault();
          pick(code);
        }}
      >
        {labels[code]}{$locale?.startsWith(code) ? " ✓" : ""}
      </HeaderPanelLink>
    {/each}
  </HeaderPanelLinks>
</HeaderAction>
