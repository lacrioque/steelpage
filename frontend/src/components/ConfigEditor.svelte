<script lang="ts">
  import { onMount } from "svelte";
  import {
    Tile,
    Button,
    TextInput,
    PasswordInput,
    NumberInput,
    Toggle,
    Select,
    SelectItem,
    Tag,
    InlineNotification,
    DataTable,
  } from "carbon-components-svelte";
  import Locked from "carbon-icons-svelte/lib/Locked.svelte";
  import Reset from "carbon-icons-svelte/lib/Reset.svelte";
  import Download from "carbon-icons-svelte/lib/Download.svelte";
  import {
    getConfigSchema,
    getConfigEffective,
    patchConfig,
    unsetConfig,
    getConfigAudit,
    exportConfigURL,
    type ConfigFieldSchema,
    type ConfigFieldState,
    type ConfigAuditEntry,
  } from "../lib/config-api";
  import { _ } from "../lib/i18n";

  let schema: ConfigFieldSchema[] = [];
  let state: ConfigFieldState[] = [];
  let audit: ConfigAuditEntry[] = [];
  let loading = true;
  let error = "";

  // Per-field draft input. Keyed by field.key.
  let drafts: Record<string, unknown> = {};
  let busy: Record<string, boolean> = {};

  onMount(refresh);

  async function refresh() {
    loading = true;
    error = "";
    try {
      [schema, state, audit] = await Promise.all([
        getConfigSchema(),
        getConfigEffective(),
        getConfigAudit(50),
      ]);
      seedDraftsFromState();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      loading = false;
    }
  }

  function seedDraftsFromState() {
    drafts = {};
    for (const f of schema) {
      const s = state.find((x) => x.key === f.key);
      if (!s) continue;
      if (f.sensitive) {
        // Sensitive draft starts empty — empty submit keeps existing value.
        drafts[f.key] = "";
      } else {
        drafts[f.key] = s.value as unknown;
      }
    }
  }

  // Group schema for layout. Sort groups by first-seen order; within each,
  // sort fields by order.
  $: groups = groupSchema(schema);

  function groupSchema(items: ConfigFieldSchema[]): { name: string; fields: ConfigFieldSchema[] }[] {
    const map = new Map<string, ConfigFieldSchema[]>();
    for (const f of items) {
      const arr = map.get(f.group) ?? [];
      arr.push(f);
      map.set(f.group, arr);
    }
    return [...map.entries()].map(([name, fields]) => ({
      name,
      fields: fields.sort((a, b) => a.order - b.order),
    }));
  }

  function fieldState(key: string): ConfigFieldState | undefined {
    return state.find((s) => s.key === key);
  }

  async function save(field: ConfigFieldSchema) {
    if (busy[field.key]) return;
    busy[field.key] = true;
    error = "";
    try {
      const raw = drafts[field.key];
      let value: unknown = raw;
      if (field.type === "int" && typeof raw === "string") {
        value = parseInt(raw, 10);
      }
      if (field.type === "string_slice" && typeof raw === "string") {
        value = raw.split(",").map((s) => s.trim()).filter(Boolean);
      }
      // Sensitive: skip the patch entirely when the input is empty.
      if (field.sensitive && (value === "" || value == null)) {
        return;
      }
      state = await patchConfig(field.key, value);
      audit = await getConfigAudit(50);
      seedDraftsFromState();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      busy[field.key] = false;
      busy = busy;
    }
  }

  async function revert(field: ConfigFieldSchema) {
    if (busy[field.key]) return;
    if (!confirm($_("config.revert_confirm", { values: { key: field.key } }))) return;
    busy[field.key] = true;
    try {
      state = await unsetConfig(field.key);
      audit = await getConfigAudit(50);
      seedDraftsFromState();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      busy[field.key] = false;
      busy = busy;
    }
  }

  function shortKey(key: string): string {
    const parts = key.split(".");
    return parts[parts.length - 1].replace(/_/g, " ");
  }
</script>

<section class="config-editor">
  <div class="header-row">
    <h3>{$_("config.heading")}</h3>
    <a href={exportConfigURL()} class="export-link">
      <Button kind="ghost" size="sm" icon={Download}>
        {$_("config.export")}
      </Button>
    </a>
  </div>
  <p class="dim">{$_("config.intro")}</p>

  {#if error}
    <InlineNotification kind="error" title={$_("admin.error")} subtitle={error} lowContrast />
  {/if}

  {#if loading}
    <p class="dim">{$_("app.loading")}</p>
  {:else}
    {#each groups as group (group.name)}
      <Tile light class="group-tile">
        <h4>{group.name}</h4>
        <div class="fields">
          {#each group.fields as field (field.key)}
            {@const s = fieldState(field.key)}
            <div class="field" class:readonly={field.read_only}>
              <div class="field-label">
                <label for={`f-${field.key}`}>
                  <span class="name">{shortKey(field.key)}</span>
                  <span class="key">{field.key}</span>
                </label>
                <div class="badges">
                  {#if field.read_only}
                    <Tag type="cool-gray" size="sm" icon={Locked}>{$_("config.readonly")}</Tag>
                  {/if}
                  {#if s?.has_override}
                    <Tag type="purple" size="sm">{$_("config.override")}</Tag>
                  {:else if !field.read_only}
                    <Tag type="gray" size="sm">{$_("config.from_yaml")}</Tag>
                  {/if}
                  {#if field.sensitive && s?.has_value}
                    <Tag type="green" size="sm">{$_("config.configured")}</Tag>
                  {/if}
                </div>
              </div>

              <div class="field-input">
                {#if field.read_only}
                  <code class="readonly-value">
                    {#if field.sensitive}
                      {s?.has_value ? "•••••• (set)" : "(empty)"}
                    {:else if Array.isArray(s?.value)}
                      {(s?.value as string[]).join(", ") || "(empty)"}
                    {:else if s?.value === "" || s?.value == null}
                      (empty)
                    {:else}
                      {String(s?.value)}
                    {/if}
                  </code>
                {:else if field.type === "bool"}
                  <Toggle
                    labelA={$_("config.off")}
                    labelB={$_("config.on")}
                    bind:toggled={drafts[field.key] as boolean}
                    on:toggle={() => save(field)}
                  />
                {:else if field.type === "enum"}
                  <Select
                    inline
                    hideLabel
                    labelText={field.key}
                    bind:selected={drafts[field.key] as string}
                  >
                    {#each field.enum ?? [] as opt (opt)}
                      <SelectItem value={opt} text={opt} />
                    {/each}
                  </Select>
                  <Button size="sm" disabled={busy[field.key]} on:click={() => save(field)}>
                    {$_("config.save")}
                  </Button>
                {:else if field.type === "int"}
                  <NumberInput
                    hideLabel
                    label={field.key}
                    bind:value={drafts[field.key] as number}
                    min={field.min}
                    max={field.max}
                  />
                  <Button size="sm" disabled={busy[field.key]} on:click={() => save(field)}>
                    {$_("config.save")}
                  </Button>
                {:else if field.sensitive}
                  <PasswordInput
                    hideLabel
                    labelText={field.key}
                    placeholder={s?.has_value ? $_("config.sensitive_placeholder_set") : $_("config.sensitive_placeholder_empty")}
                    bind:value={drafts[field.key] as string}
                  />
                  <Button size="sm" disabled={busy[field.key] || !drafts[field.key]} on:click={() => save(field)}>
                    {$_("config.save")}
                  </Button>
                {:else if field.type === "string_slice"}
                  <TextInput
                    hideLabel
                    labelText={field.key}
                    placeholder="value1, value2, value3"
                    bind:value={drafts[field.key] as string}
                  />
                  <Button size="sm" disabled={busy[field.key]} on:click={() => save(field)}>
                    {$_("config.save")}
                  </Button>
                {:else}
                  <TextInput
                    hideLabel
                    labelText={field.key}
                    bind:value={drafts[field.key] as string}
                  />
                  <Button size="sm" disabled={busy[field.key]} on:click={() => save(field)}>
                    {$_("config.save")}
                  </Button>
                {/if}

                {#if !field.read_only && s?.has_override}
                  <Button kind="ghost" size="sm" icon={Reset} on:click={() => revert(field)}>
                    {$_("config.revert")}
                  </Button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </Tile>
    {/each}

    <h4 class="sub-heading">{$_("config.history_heading")}</h4>
    {#if audit.length === 0}
      <p class="dim">{$_("config.history_empty")}</p>
    {:else}
      <DataTable
        size="short"
        headers={[
          { key: "at", value: $_("config.history_when") },
          { key: "actor", value: $_("config.history_who") },
          { key: "key", value: $_("config.history_key") },
          { key: "change", value: $_("config.history_change") },
        ]}
        rows={audit.map((e) => ({
          id: String(e.id),
          at: new Date(e.at).toLocaleString(),
          actor: e.actor_display ?? "—",
          key: e.key,
          change: `${e.old_value ?? "(none)"} → ${e.new_value ?? "(default)"}`,
        }))}
      />
    {/if}
  {/if}
</section>

<style>
  .config-editor {
    margin-top: 2rem;
  }
  .header-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }
  .header-row h3 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0;
  }
  .export-link {
    text-decoration: none;
  }
  .dim {
    color: #6f6a60;
    font-size: 0.9rem;
  }
  :global(.group-tile) {
    margin: 1rem 0;
  }
  .config-editor h4 {
    margin: 0 0 0.75rem;
    font-size: 0.95rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #525252;
  }
  .sub-heading {
    margin-top: 2rem !important;
  }
  .fields {
    display: grid;
    gap: 1rem;
  }
  .field {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: 1rem;
    align-items: start;
    padding: 0.75rem 0;
    border-top: 1px solid #f4f4f4;
  }
  .field:first-child {
    border-top: 0;
  }
  .field.readonly {
    opacity: 0.8;
  }
  .field-label {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .field-label .name {
    font-weight: 500;
    text-transform: capitalize;
  }
  .field-label .key {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.75rem;
    color: #6f6a60;
  }
  .badges {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
  }
  .field-input {
    display: flex;
    gap: 0.5rem;
    align-items: end;
    flex-wrap: wrap;
  }
  .readonly-value {
    background: #f4f4f4;
    padding: 0.4rem 0.6rem;
    border-radius: 0.25rem;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.85rem;
    color: #393939;
    display: block;
  }
  @media (max-width: 768px) {
    .field {
      grid-template-columns: 1fr;
    }
  }
</style>
