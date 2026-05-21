<script lang="ts">
  import { onMount } from "svelte";
  import {
    DataTable,
    Button,
    TextInput,
    Tag,
    InlineNotification,
    Modal,
    Select,
    SelectItem,
    Tabs,
    Tab,
    TabContent,
    StructuredList,
    StructuredListHead,
    StructuredListBody,
    StructuredListRow,
    StructuredListCell,
  } from "carbon-components-svelte";
  import TrashCan from "carbon-icons-svelte/lib/TrashCan.svelte";
  import Add from "carbon-icons-svelte/lib/Add.svelte";
  import {
    listUsers,
    setUserRole,
    adminDisableUserMFA,
    listGroups,
    createGroup,
    deleteGroup,
    addGroupMember,
    removeGroupMember,
    listPermissions,
    createPermission,
    deletePermission,
    effectivePermissions,
    gitStatus,
    gitPull,
    gitPush,
    gitAbort,
    mailerStatus,
    sendTestMail,
    type Group,
    type PermissionRule,
    type GitStatus,
    type GitSyncResult,
    type MailerStatus,
  } from "../lib/admin-api";
  import { getCapabilities, type AuthCapabilities } from "../lib/auth-api";
  import type { Me } from "../lib/identity";
  import ConfigEditor from "../components/ConfigEditor.svelte";
  import { _ } from "../lib/i18n";

  let active = 0;

  let users: Me[] = [];
  let groups: Group[] = [];
  let rules: PermissionRule[] = [];
  let caps: AuthCapabilities | null = null;
  let error = "";

  let newGroupName = "";
  let newGroupDesc = "";
  let createBusy = false;

  let memberDialogOpen = false;
  let memberDialogGroup: Group | null = null;
  let memberDialogUserID: number | "" = "";

  let newRuleGlob = "";
  let newRuleSubjectType: PermissionRule["subject_type"] = "authenticated";
  let newRuleSubjectValue = "";
  let newRulePermission: PermissionRule["permission"] = "read";
  let newRuleBusy = false;

  let testPath = "";
  let testResults: PermissionRule[] = [];

  let git: GitStatus | null = null;
  let gitBusy = false;
  let lastSync: GitSyncResult | null = null;

  let smtp: MailerStatus | null = null;
  let smtpBusy = false;
  let smtpTestResult: { ok: boolean; message: string } | null = null;
  let smtpTo = "";

  onMount(refresh);

  async function refresh() {
    error = "";
    try {
      [users, groups, rules, caps, git, smtp] = await Promise.all([
        listUsers(),
        listGroups(),
        listPermissions(),
        getCapabilities(),
        gitStatus(),
        mailerStatus(),
      ]);
      lastSync = git?.last_sync ?? null;
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function refreshGit() {
    try {
      git = await gitStatus();
      lastSync = git?.last_sync ?? null;
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function runGitPull() {
    if (gitBusy) return;
    gitBusy = true;
    try {
      const res = await gitPull();
      git = res.status;
      lastSync = git?.last_sync ?? lastSync;
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      gitBusy = false;
    }
  }

  async function runGitPush() {
    if (gitBusy) return;
    gitBusy = true;
    try {
      const res = await gitPush();
      git = res.status;
      lastSync = res.sync;
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      gitBusy = false;
    }
  }

  async function runSmtpTest() {
    if (smtpBusy) return;
    smtpBusy = true;
    smtpTestResult = null;
    try {
      const res = await sendTestMail(smtpTo.trim() || undefined);
      smtpTestResult = { ok: true, message: `Sent to ${res.to}` };
    } catch (err) {
      smtpTestResult = { ok: false, message: err instanceof Error ? err.message : "" };
    } finally {
      smtpBusy = false;
    }
  }

  async function runGitAbort() {
    if (gitBusy) return;
    if (!confirm($_("admin.git_abort_confirm"))) return;
    gitBusy = true;
    try {
      const res = await gitAbort();
      git = res.status;
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      gitBusy = false;
    }
  }

  async function changeRole(id: number, role: "admin" | "user") {
    try {
      await setUserRole(id, role);
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function emergencyDisableMFA(id: number, displayName: string) {
    if (!confirm($_("admin.disable_mfa_confirm", { values: { name: displayName } }))) return;
    try {
      await adminDisableUserMFA(id);
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function submitGroup() {
    if (!newGroupName.trim()) return;
    createBusy = true;
    try {
      await createGroup(newGroupName.trim(), newGroupDesc.trim());
      newGroupName = "";
      newGroupDesc = "";
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      createBusy = false;
    }
  }

  async function removeGroup(g: Group) {
    if (!confirm($_("admin.confirm_delete_group", { values: { name: g.name } }))) return;
    try {
      await deleteGroup(g.id);
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  function userLabel(id: number): string {
    const u = users.find((x) => x.id === id);
    return u ? `${u.display_name} (${u.email ?? "—"})` : `#${id}`;
  }

  function openMemberDialog(g: Group) {
    memberDialogGroup = g;
    memberDialogUserID = "";
    memberDialogOpen = true;
  }

  async function submitMember() {
    if (memberDialogGroup == null || memberDialogUserID === "") return;
    try {
      await addGroupMember(memberDialogGroup.id, Number(memberDialogUserID));
      memberDialogOpen = false;
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function dropMember(g: Group, userID: number) {
    try {
      await removeGroupMember(g.id, userID);
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function addRule() {
    if (!newRuleGlob.trim()) return;
    const subjectValue =
      newRuleSubjectType === "anonymous" || newRuleSubjectType === "authenticated"
        ? ""
        : newRuleSubjectValue.trim();
    if (
      (newRuleSubjectType === "role" ||
        newRuleSubjectType === "group" ||
        newRuleSubjectType === "user") &&
      !subjectValue
    ) {
      error = "subject_value is required for this subject type";
      return;
    }
    newRuleBusy = true;
    try {
      await createPermission({
        path_glob: newRuleGlob.trim(),
        subject_type: newRuleSubjectType,
        subject_value: subjectValue,
        permission: newRulePermission,
      });
      newRuleGlob = "";
      newRuleSubjectValue = "";
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    } finally {
      newRuleBusy = false;
    }
  }

  async function dropRule(r: PermissionRule) {
    if (!confirm($_("admin.confirm_delete_rule", { values: { id: r.id } }))) return;
    try {
      await deletePermission(r.id);
      await refresh();
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  async function runTest() {
    if (!testPath.trim()) {
      testResults = [];
      return;
    }
    try {
      testResults = await effectivePermissions(testPath.trim());
    } catch (err) {
      error = err instanceof Error ? err.message : "";
    }
  }

  function ruleSubjectLabel(r: PermissionRule): string {
    if (r.subject_type === "anonymous" || r.subject_type === "authenticated") {
      return r.subject_type;
    }
    return `${r.subject_type}:${r.subject_value}`;
  }
</script>

<section class="admin">
  <h1>{$_("admin.heading")}</h1>

  {#if error}
    <InlineNotification kind="error" title={$_("admin.error")} subtitle={error} lowContrast />
  {/if}

  <Tabs bind:selected={active}>
    <Tab label={$_("admin.tab_users")} />
    <Tab label={$_("admin.tab_groups")} />
    <Tab label={$_("admin.tab_permissions")} />
    <Tab label={$_("admin.tab_settings")} />

    <svelte:fragment slot="content">
      <TabContent>
        <DataTable
          headers={[
            { key: "display_name", value: $_("admin.col_name") },
            { key: "email", value: $_("admin.col_email") },
            { key: "role", value: $_("admin.col_role") },
            { key: "groups", value: $_("admin.col_groups") },
            { key: "mfa", value: $_("admin.col_mfa") },
          ]}
          rows={users.map((u) => ({
            id: String(u.id),
            display_name: u.display_name,
            email: u.email ?? "—",
            role: u.role,
            groups: (u.groups ?? []).join(", "),
            mfa: u.totp_enabled_at ? "on" : "off",
            _user: u,
          }))}
        >
          <svelte:fragment slot="cell" let:row let:cell>
            {#if cell.key === "role"}
              <Select
                inline
                hideLabel
                labelText={$_("admin.col_role")}
                selected={cell.value}
                on:change={(e) =>
                  changeRole(Number(row.id), (e.target as HTMLSelectElement).value as "admin" | "user")}
              >
                <SelectItem value="user" text={$_("admin.role_user")} />
                <SelectItem value="admin" text={$_("admin.role_admin")} />
              </Select>
            {:else if cell.key === "mfa"}
              {#if cell.value === "on"}
                <Tag type="green" size="sm">{$_("admin.mfa_on")}</Tag>
                <Button
                  kind="danger-ghost"
                  size="sm"
                  on:click={() => emergencyDisableMFA(Number(row.id), (row as any)._user.display_name)}
                >
                  {$_("admin.disable_mfa")}
                </Button>
              {:else}
                <Tag type="cool-gray" size="sm">{$_("admin.mfa_off")}</Tag>
              {/if}
            {:else}
              {cell.value}
            {/if}
          </svelte:fragment>
        </DataTable>
      </TabContent>

      <TabContent>
        <form on:submit|preventDefault={submitGroup} class="create-group">
          <TextInput
            labelText={$_("admin.new_group_name")}
            placeholder={$_("admin.new_group_name_placeholder")}
            bind:value={newGroupName}
          />
          <TextInput
            labelText={$_("admin.new_group_description")}
            placeholder={$_("admin.new_group_description_placeholder")}
            bind:value={newGroupDesc}
          />
          <Button type="submit" icon={Add} disabled={createBusy || !newGroupName.trim()}>
            {$_("admin.create_group")}
          </Button>
        </form>

        <div class="groups-list">
          {#each groups as g (g.id)}
            <article class="group-card">
              <header>
                <strong>{g.name}</strong>
                {#if g.description}<span class="dim">{g.description}</span>{/if}
                <span style="flex:1"></span>
                <Button kind="ghost" size="sm" icon={Add} on:click={() => openMemberDialog(g)}>
                  {$_("admin.add_member")}
                </Button>
                <Button kind="danger-ghost" size="sm" icon={TrashCan} on:click={() => removeGroup(g)}>
                  {$_("admin.delete_group")}
                </Button>
              </header>
              <div class="members">
                {#if (g.members ?? []).length === 0}
                  <span class="dim">{$_("admin.empty_group")}</span>
                {:else}
                  {#each g.members ?? [] as uid (uid)}
                    <Tag filter on:close={() => dropMember(g, uid)}>{userLabel(uid)}</Tag>
                  {/each}
                {/if}
              </div>
            </article>
          {/each}
        </div>
      </TabContent>

      <TabContent>
        <p class="dim">{$_("admin.permissions_hint")}</p>

        <form on:submit|preventDefault={addRule} class="create-rule">
          <TextInput
            labelText={$_("admin.rule_glob")}
            placeholder={$_("admin.rule_glob_placeholder")}
            bind:value={newRuleGlob}
          />
          <Select labelText={$_("admin.rule_subject_type")} bind:selected={newRuleSubjectType}>
            <SelectItem value="anonymous" text={$_("admin.subject_anonymous")} />
            <SelectItem value="authenticated" text={$_("admin.subject_authenticated")} />
            <SelectItem value="role" text={$_("admin.subject_role")} />
            <SelectItem value="group" text={$_("admin.subject_group")} />
            <SelectItem value="user" text={$_("admin.subject_user")} />
          </Select>
          {#if newRuleSubjectType === "role" || newRuleSubjectType === "group" || newRuleSubjectType === "user"}
            <TextInput
              labelText={$_("admin.rule_subject_value")}
              placeholder={
                newRuleSubjectType === "role"
                  ? $_("admin.rule_subject_value_role")
                  : newRuleSubjectType === "group"
                  ? $_("admin.rule_subject_value_group")
                  : $_("admin.rule_subject_value_user")
              }
              bind:value={newRuleSubjectValue}
            />
          {:else}
            <div></div>
          {/if}
          <Select labelText={$_("admin.rule_permission")} bind:selected={newRulePermission}>
            <SelectItem value="read" text={$_("admin.permission_read")} />
            <SelectItem value="comment" text={$_("admin.permission_comment")} />
            <SelectItem value="write" text={$_("admin.permission_write")} />
          </Select>
          <Button type="submit" icon={Add} disabled={newRuleBusy || !newRuleGlob.trim()}>
            {$_("admin.create_rule")}
          </Button>
        </form>

        <DataTable
          headers={[
            { key: "path_glob", value: $_("admin.rule_glob") },
            { key: "subject", value: $_("admin.rule_subject") },
            { key: "permission", value: $_("admin.rule_permission") },
            { key: "actions", value: "" },
          ]}
          rows={rules.map((r) => ({
            id: String(r.id),
            path_glob: r.path_glob,
            subject: ruleSubjectLabel(r),
            permission: r.permission,
            _rule: r,
          }))}
        >
          <svelte:fragment slot="cell" let:row let:cell>
            {#if cell.key === "actions"}
              <Button
                kind="danger-ghost"
                size="sm"
                icon={TrashCan}
                on:click={() => dropRule((row as any)._rule)}
              >
                {$_("admin.delete_rule")}
              </Button>
            {:else}
              {cell.value}
            {/if}
          </svelte:fragment>
        </DataTable>

        <h3 class="sub-heading">{$_("admin.test_path_heading")}</h3>
        <form on:submit|preventDefault={runTest} class="test-path">
          <TextInput
            labelText={$_("admin.test_path_label")}
            placeholder={$_("admin.test_path_placeholder")}
            bind:value={testPath}
          />
          <Button type="submit">{$_("admin.test_path_button")}</Button>
        </form>

        {#if testPath && testResults.length === 0}
          <p class="dim">{$_("admin.test_no_rules")}</p>
        {:else if testResults.length > 0}
          <ul class="test-results">
            {#each testResults as r (r.id)}
              <li>
                <code>{r.path_glob}</code> · <strong>{ruleSubjectLabel(r)}</strong> · {r.permission}
              </li>
            {/each}
          </ul>
        {/if}
      </TabContent>

      <TabContent>
        <h3 class="sub-heading">{$_("admin.git_heading")}</h3>
        {#if git}
          {#if !git.has_remote}
            <InlineNotification
              kind="info"
              title={$_("admin.git_no_remote_title")}
              subtitle={$_("admin.git_no_remote_hint")}
              lowContrast
              hideCloseButton
            />
          {:else if git.rebase_in_progress}
            <InlineNotification
              kind="error"
              title={$_("admin.git_conflict_title")}
              subtitle={$_("admin.git_conflict_hint")}
              lowContrast
              hideCloseButton
            />
            {#if (git.conflict_files ?? []).length > 0}
              <ul class="conflict-files">
                {#each git.conflict_files ?? [] as f (f)}
                  <li><code>{f}</code></li>
                {/each}
              </ul>
            {/if}
          {/if}

          <div class="git-status">
            <div>
              <strong>{$_("admin.git_branch")}</strong>
              <span class="dim">{git.branch}</span>
            </div>
            <div>
              <strong>{$_("admin.git_remote")}</strong>
              <span class="dim">{git.remote}</span>
            </div>
            <div>
              <strong>{$_("admin.git_ahead")}</strong>
              <span class="dim">{git.ahead}</span>
            </div>
            <div>
              <strong>{$_("admin.git_behind")}</strong>
              <span class="dim">{git.behind}</span>
            </div>
          </div>

          <div class="git-actions">
            <Button kind="secondary" on:click={runGitPull} disabled={gitBusy || !git.has_remote}>
              {$_("admin.git_pull")}
            </Button>
            <Button on:click={runGitPush} disabled={gitBusy || !git.has_remote}>
              {$_("admin.git_push")}
            </Button>
            {#if git.rebase_in_progress}
              <Button kind="danger" on:click={runGitAbort} disabled={gitBusy}>
                {$_("admin.git_abort")}
              </Button>
            {/if}
            <Button kind="ghost" on:click={refreshGit} disabled={gitBusy}>
              {$_("admin.git_refresh")}
            </Button>
          </div>

          {#if lastSync}
            <div class="git-last-sync">
              <strong>{$_("admin.git_last_sync")}</strong>
              <span class="dim">{new Date(lastSync.at).toLocaleString()}</span>
              {#if lastSync.conflict}
                <span class="status-conflict">{$_("admin.git_status_conflict")}</span>
              {:else if lastSync.error}
                <span class="status-error">{$_("admin.git_status_error")}: {lastSync.error}</span>
              {:else if lastSync.pushed}
                <span class="status-ok">{$_("admin.git_status_pushed")}</span>
              {:else if lastSync.pulled}
                <span class="status-ok">{$_("admin.git_status_pulled")}</span>
              {/if}
            </div>
          {/if}
        {/if}

        <h3 class="sub-heading">{$_("admin.mailer_heading")}</h3>
        {#if smtp}
          {#if !smtp.enabled}
            <InlineNotification
              kind="info"
              title={$_("admin.mailer_not_configured_title")}
              subtitle={$_("admin.mailer_not_configured_hint")}
              lowContrast
              hideCloseButton
            />
          {:else}
            <div class="git-status">
              <div>
                <strong>{$_("admin.mailer_host")}</strong>
                <span class="dim">{smtp.host}:{smtp.port}</span>
              </div>
              <div>
                <strong>{$_("admin.mailer_encryption")}</strong>
                <span class="dim">{smtp.encryption}</span>
              </div>
              <div>
                <strong>{$_("admin.mailer_from")}</strong>
                <span class="dim">
                  {#if smtp.from_name}{smtp.from_name} &lt;{smtp.from_address}&gt;{:else}{smtp.from_address}{/if}
                </span>
              </div>
            </div>

            <div class="smtp-test">
              <TextInput
                labelText={$_("admin.mailer_test_to")}
                placeholder={$_("admin.mailer_test_to_placeholder")}
                bind:value={smtpTo}
              />
              <Button on:click={runSmtpTest} disabled={smtpBusy}>
                {smtpBusy ? $_("admin.mailer_test_sending") : $_("admin.mailer_test_button")}
              </Button>
            </div>

            {#if smtpTestResult}
              <InlineNotification
                kind={smtpTestResult.ok ? "success" : "error"}
                title={smtpTestResult.ok ? $_("admin.mailer_test_ok_title") : $_("admin.mailer_test_failed_title")}
                subtitle={smtpTestResult.message}
                lowContrast
                hideCloseButton
              />
            {/if}
          {/if}
        {/if}

        <ConfigEditor />

        <h3 class="sub-heading">{$_("admin.config_heading")}</h3>
        <p class="dim">{$_("admin.settings_intro")}</p>

        {#if caps}
          <StructuredList>
            <StructuredListHead>
              <StructuredListRow head>
                <StructuredListCell head>{$_("admin.settings_key")}</StructuredListCell>
                <StructuredListCell head>{$_("admin.settings_value")}</StructuredListCell>
              </StructuredListRow>
            </StructuredListHead>
            <StructuredListBody>
              <StructuredListRow>
                <StructuredListCell>{$_("admin.settings_local_enabled")}</StructuredListCell>
                <StructuredListCell>
                  <code>{caps.local_enabled}</code>
                </StructuredListCell>
              </StructuredListRow>
              <StructuredListRow>
                <StructuredListCell>{$_("admin.settings_anon_read")}</StructuredListCell>
                <StructuredListCell>
                  <code>{caps.allow_anonymous_read}</code>
                </StructuredListCell>
              </StructuredListRow>
              <StructuredListRow>
                <StructuredListCell>{$_("admin.settings_providers")}</StructuredListCell>
                <StructuredListCell>
                  {#if caps.providers.length === 0}
                    <span class="dim">{$_("admin.settings_no_providers")}</span>
                  {:else}
                    {#each caps.providers as p (p.name)}
                      <Tag>{p.label}</Tag>
                    {/each}
                  {/if}
                </StructuredListCell>
              </StructuredListRow>
            </StructuredListBody>
          </StructuredList>
        {/if}
      </TabContent>
    </svelte:fragment>
  </Tabs>
</section>

<Modal
  bind:open={memberDialogOpen}
  modalHeading={memberDialogGroup ? $_("admin.add_member_to", { values: { name: memberDialogGroup.name } }) : ""}
  primaryButtonText={$_("admin.add_member")}
  secondaryButtonText={$_("admin.cancel")}
  primaryButtonDisabled={memberDialogUserID === ""}
  on:submit={submitMember}
  on:click:button--secondary={() => (memberDialogOpen = false)}
>
  <Select labelText={$_("admin.col_name")} bind:selected={memberDialogUserID}>
    <SelectItem value="" text={$_("admin.pick_user")} disabled />
    {#each users as u (u.id)}
      <SelectItem value={String(u.id)} text={userLabel(u.id)} />
    {/each}
  </Select>
</Modal>

<style>
  .admin {
    max-width: 1440px;
    margin: 0 auto;
    padding: 1rem 1rem 3rem;
  }
  .admin h1 {
    font-weight: 600;
    margin: 0.5rem 0 1.5rem;
  }
  .sub-heading {
    font-size: 1rem;
    font-weight: 600;
    margin: 1.5rem 0 0.6rem;
  }
  .create-group {
    display: grid;
    grid-template-columns: 2fr 3fr auto;
    gap: 1rem;
    align-items: end;
    margin-bottom: 1.5rem;
  }
  .create-rule {
    display: grid;
    grid-template-columns: 2fr 1.2fr 1.2fr 1fr auto;
    gap: 1rem;
    align-items: end;
    margin-bottom: 1.5rem;
  }
  .test-path {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 1rem;
    align-items: end;
    margin-bottom: 0.5rem;
  }
  .test-results {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .test-results li {
    padding: 0.25rem 0;
    border-bottom: 1px dashed #e0e0e0;
  }
  .test-results code {
    background: #f4f4f4;
    padding: 0.1rem 0.3rem;
    border-radius: 0.2rem;
  }
  .git-status {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 1rem;
    padding: 1rem;
    background: #f4f4f4;
    border-radius: 0.4rem;
    margin: 0.75rem 0 1rem;
  }
  .git-status strong {
    display: block;
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #525252;
    margin-bottom: 0.25rem;
  }
  .git-actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-bottom: 1rem;
  }
  .git-last-sync {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-wrap: wrap;
    font-size: 0.9rem;
    padding-bottom: 1rem;
  }
  .status-ok {
    color: #105a2b;
  }
  .status-conflict,
  .status-error {
    color: #9b1c1c;
  }
  .smtp-test {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 1rem;
    align-items: end;
    margin: 1rem 0;
  }
  .conflict-files {
    list-style: none;
    padding: 0.5rem 1rem;
    margin: 0 0 1rem;
    background: #fde8e8;
    border-radius: 0.4rem;
  }
  .conflict-files code {
    background: transparent;
  }
  .groups-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .group-card {
    padding: 0.75rem 1rem;
    border: 1px solid #e0e0e0;
    border-radius: 0.4rem;
    background: white;
  }
  .group-card header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.5rem;
  }
  .members {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }
  .dim {
    color: #6f6a60;
    font-size: 0.85rem;
  }
</style>
