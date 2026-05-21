<script lang="ts">
  import "carbon-components-svelte/css/white.css";
  import { isLoading } from "svelte-i18n";
  import "./lib/i18n";
  import CarbonShell from "./routes/CarbonShell.svelte";
  import "./styles/app.css";
  import "./styles/print.css";
</script>

{#if !$isLoading}
  <CarbonShell />
{/if}
