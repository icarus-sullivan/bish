<script lang="ts">
  import { commandCenter, projectRoot } from '../lib/stores'
  import {
    GetCommandCenterBranches, SetCommandCenterTarget, SaveCommandCenterDefinition,
    StartCommandCenterRepo, StopCommandCenterRepo, StartCommandCenterService,
    StartAllCommandCenter, StopAllCommandCenter, RefreshCommandCenterRepo,
  } from '../lib/wails'
  import type { CCRepo, CCTarget, CCStep, CCBranchInfo, CCDefinition } from '../lib/wails'
  import { IconPlus, IconPlayerPlayFilled, IconPlayerStopFilled, IconRefresh, IconTrash, IconChevronDown, IconExternalLink } from '@tabler/icons-svelte'
  import { modalA11y } from '../lib/a11y'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

  let branchesCache: Record<string, CCBranchInfo[]> = $state({})
  let envDraft: Record<string, string> = $state({})

  let addServiceFor: string | null = $state(null)
  let addServiceName = $state('')
  let addServiceCmd = $state('')
  let addServicePort = $state('')

  let addStepFor: string | null = $state(null)
  let addStepName = $state('')
  let addStepCmd = $state('')
  let addStepDefault = $state(false)
  let addStepDestructive = $state(false)

  // lazy-load branches + seed the env textarea draft per repo, once — guarded
  // so this never fights the user's typing or re-fetches on every 2s snapshot
  $effect(() => {
    for (const r of $commandCenter.definition.repos) {
      if (!(r.id in branchesCache)) {
        branchesCache[r.id] = []
        GetCommandCenterBranches(r.id).then(b => { branchesCache[r.id] = b ?? [] }).catch(() => {})
      }
      const t = $commandCenter.state.targets[r.id]
      if (t && !(r.id in envDraft)) envDraft[r.id] = envToText(t.env)
    }
  })

  function envToText(env?: Record<string, string>) {
    return Object.entries(env ?? {}).map(([k, v]) => `${k}=${v}`).join('\n')
  }
  function parseEnvText(text: string): Record<string, string> {
    const out: Record<string, string> = {}
    for (const line of text.split('\n')) {
      const t = line.trim()
      if (!t || t.startsWith('#')) continue
      const i = t.indexOf('=')
      if (i === -1) continue
      out[t.slice(0, i).trim()] = t.slice(i + 1).trim()
    }
    return out
  }

  function stepEnabled(target: CCTarget, step: CCStep) {
    return target.steps?.[step.name] ?? step.default
  }

  async function toggleService(repo: CCRepo, target: CCTarget, name: string) {
    const current = target.services ?? []
    const services = current.includes(name)
      ? current.filter(s => s !== name)
      : [...current, name]
    await SetCommandCenterTarget(repo.id, { ...target, services })
  }

  async function toggleStep(repo: CCRepo, target: CCTarget, step: CCStep) {
    const enabled = stepEnabled(target, step)
    if (!enabled && step.destructive && !confirm(`Enable "${step.name}"? This step can discard data.`)) return
    await SetCommandCenterTarget(repo.id, { ...target, steps: { ...(target.steps ?? {}), [step.name]: !enabled } })
  }

  async function setMode(repo: CCRepo, target: CCTarget, mode: string) {
    await SetCommandCenterTarget(repo.id, { ...target, mode: mode as CCTarget['mode'] })
  }

  async function setBranch(repo: CCRepo, target: CCTarget, branch: string) {
    await SetCommandCenterTarget(repo.id, { ...target, branch })
  }

  async function saveEnv(repo: CCRepo, target: CCTarget) {
    await SetCommandCenterTarget(repo.id, { ...target, env: parseEnvText(envDraft[repo.id] ?? '') })
  }

  async function toggleDependsOn(def: CCDefinition, repo: CCRepo, depId: string) {
    const current = repo.dependsOn ?? []
    const dependsOn = current.includes(depId)
      ? current.filter(d => d !== depId)
      : [...current, depId]
    await SaveCommandCenterDefinition({ repos: def.repos.map(r => r.id === repo.id ? { ...r, dependsOn } : r) })
  }

  function openAddService(repoId: string) {
    addServiceFor = repoId
    addServiceName = ''; addServiceCmd = ''; addServicePort = ''
  }
  async function submitAddService(def: CCDefinition) {
    if (!addServiceFor || !addServiceName.trim() || !addServiceCmd.trim()) return
    const port = parseInt(addServicePort, 10) || 0
    const repos = def.repos.map(r => r.id === addServiceFor
      ? { ...r, services: [...(r.services ?? []), { name: addServiceName.trim(), cmd: addServiceCmd.trim(), port }] }
      : r)
    await SaveCommandCenterDefinition({ repos })
    addServiceFor = null
  }
  function removeService(def: CCDefinition, repo: CCRepo, name: string) {
    SaveCommandCenterDefinition({ repos: def.repos.map(r => r.id === repo.id ? { ...r, services: (r.services ?? []).filter(s => s.name !== name) } : r) })
  }

  function openAddStep(repoId: string) {
    addStepFor = repoId
    addStepName = ''; addStepCmd = ''; addStepDefault = false; addStepDestructive = false
  }
  async function submitAddStep(def: CCDefinition) {
    if (!addStepFor || !addStepName.trim() || !addStepCmd.trim()) return
    const repos = def.repos.map(r => r.id === addStepFor
      ? { ...r, steps: [...(r.steps ?? []), { name: addStepName.trim(), cmd: addStepCmd.trim(), default: addStepDefault, destructive: addStepDestructive }] }
      : r)
    await SaveCommandCenterDefinition({ repos })
    addStepFor = null
  }
  function removeStep(def: CCDefinition, repo: CCRepo, name: string) {
    SaveCommandCenterDefinition({ repos: def.repos.map(r => r.id === repo.id ? { ...r, steps: (r.steps ?? []).filter(s => s.name !== name) } : r) })
  }

  function statusFor(repoId: string, key: string) {
    return $commandCenter.statuses[repoId + '|' + key]
  }

  function openPort(e: MouseEvent, port: number) {
    e.stopPropagation()
    BrowserOpenURL(`http://localhost:${port}`)
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Command Center</span>
    <div class="header-right">
      <button class="hdr-btn" onclick={() => StartAllCommandCenter()} title="Start all"><IconPlayerPlayFilled size={13} /></button>
      <button class="hdr-btn" onclick={() => StopAllCommandCenter()} title="Stop all"><IconPlayerStopFilled size={13} /></button>
    </div>
  </div>
  <div class="list">
    {#if !$projectRoot}
      <div class="empty">open a project to use Command Center</div>
    {:else if $commandCenter.definition.repos.length === 0}
      <div class="empty">no git repos found in this workspace</div>
    {:else}
      {#each $commandCenter.definition.repos as repo (repo.id)}
        {@const target = $commandCenter.state.targets[repo.id]}
        {#if target}
          <div class="card">
            <div class="card-header">
              <span class="repo-name">{repo.name}</span>
              {#if repo.dependsOn?.length}
                <span class="dep-badge" title="depends on">→ {repo.dependsOn.join(', ')}</span>
              {/if}
              <div class="card-actions">
                <button class="hdr-btn" onclick={() => RefreshCommandCenterRepo(repo.id)} title={`Fetch ${repo.mainBranch}`}><IconRefresh size={13} /></button>
                <button class="hdr-btn" onclick={() => StartCommandCenterRepo(repo.id)} title={`Start ${repo.name}`}><IconPlayerPlayFilled size={13} /></button>
                <button class="hdr-btn" onclick={() => StopCommandCenterRepo(repo.id)} title={`Stop ${repo.name}`}><IconPlayerStopFilled size={13} /></button>
              </div>
            </div>

            <div class="checkout-row">
              <span class="select-wrap">
                <select value={target.mode} onchange={(e) => setMode(repo, target, (e.target as HTMLSelectElement).value)}>
                  <option value="off">off</option>
                  <option value="main">main checkout</option>
                  <option value="worktree">worktree</option>
                </select>
                <IconChevronDown size={13} class="select-chevron" />
              </span>
              {#if target.mode !== 'off'}
                <input class="branch-input" list={`cc-branches-${repo.id}`} placeholder={repo.mainBranch}
                  value={target.branch} onchange={(e) => setBranch(repo, target, (e.target as HTMLInputElement).value)} />
                <datalist id={`cc-branches-${repo.id}`}>
                  {#each branchesCache[repo.id] ?? [] as b}
                    <option value={b.name}></option>
                  {/each}
                </datalist>
              {/if}
            </div>

            {#if repo.services?.length}
              <div class="section-label">Services</div>
              {#each repo.services as svc (svc.name)}
                {@const st = statusFor(repo.id, svc.name)}
                <div class="row">
                  <input type="checkbox" checked={(target.services ?? []).includes(svc.name)} onchange={() => toggleService(repo, target, svc.name)} />
                  <span class="status-dot" class:running={st?.status === 'running'} class:crashed={st?.status === 'crashed'} class:stopped={st?.status === 'stopped'}></span>
                  <span class="svc-name" title={svc.cmd}>{svc.name}</span>
                  {#if svc.port}
                    <button class="badge port" onclick={(e) => openPort(e, svc.port)} title="Open http://localhost:{svc.port} in browser">
                      <IconExternalLink size={9} />:{svc.port}
                    </button>
                  {/if}
                  <button class="row-btn" onclick={() => StartCommandCenterService(repo.id, svc.name)} title="Start"><IconPlayerPlayFilled size={12} /></button>
                  <button class="row-btn" onclick={() => removeService($commandCenter.definition, repo, svc.name)} title="Remove"><IconTrash size={12} /></button>
                </div>
              {/each}
            {/if}
            <button class="add-link" onclick={() => openAddService(repo.id)}><IconPlus size={11} /> add service</button>

            {#if repo.steps?.length}
              <div class="section-label">Before start</div>
              {#each repo.steps as step (step.name)}
                <div class="row">
                  <input type="checkbox" checked={stepEnabled(target, step)} onchange={() => toggleStep(repo, target, step)} />
                  <span class="svc-name" title={step.cmd}>{step.name}</span>
                  {#if step.destructive}<span class="badge danger">destructive</span>{/if}
                  <button class="row-btn" onclick={() => removeStep($commandCenter.definition, repo, step.name)} title="Remove"><IconTrash size={12} /></button>
                </div>
              {/each}
            {/if}
            <button class="add-link" onclick={() => openAddStep(repo.id)}><IconPlus size={11} /> add step</button>

            {#if $commandCenter.definition.repos.length > 1}
              <div class="section-label">Depends on</div>
              <div class="chip-row">
                {#each $commandCenter.definition.repos.filter(r => r.id !== repo.id) as other (other.id)}
                  <button class="chip" class:active={(repo.dependsOn ?? []).includes(other.id)} onclick={() => toggleDependsOn($commandCenter.definition, repo, other.id)}>{other.name}</button>
                {/each}
              </div>
            {/if}

            <div class="section-label">Env overrides</div>
            <textarea class="env-box" rows="2" placeholder="KEY=value"
              bind:value={envDraft[repo.id]} onblur={() => saveEnv(repo, target)}></textarea>
          </div>
        {/if}
      {/each}
    {/if}
  </div>
</div>

{#if addServiceFor}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="add-overlay" onclick={() => addServiceFor = null}>
    <div class="add-panel" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={() => addServiceFor = null} onclick={(e) => e.stopPropagation()}>
      <div class="add-header">
        <span class="add-title">Add Service</span>
        <button class="add-close" onclick={() => addServiceFor = null} aria-label="Close">✕</button>
      </div>
      <div class="add-body">
        <input class="add-input" bind:value={addServiceName} placeholder="name *"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
        <input class="add-input" bind:value={addServiceCmd} placeholder="command *"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') submitAddService($commandCenter.definition) }} />
        <input class="add-input" bind:value={addServicePort} placeholder="port (optional)"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') submitAddService($commandCenter.definition) }} />
      </div>
      <div class="add-footer">
        <button class="add-btn-cancel" onclick={() => addServiceFor = null}>Cancel</button>
        <button class="add-btn-submit" onclick={() => submitAddService($commandCenter.definition)}>Add</button>
      </div>
    </div>
  </div>
{/if}

{#if addStepFor}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="add-overlay" onclick={() => addStepFor = null}>
    <div class="add-panel" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={() => addStepFor = null} onclick={(e) => e.stopPropagation()}>
      <div class="add-header">
        <span class="add-title">Add Step</span>
        <button class="add-close" onclick={() => addStepFor = null} aria-label="Close">✕</button>
      </div>
      <div class="add-body">
        <input class="add-input" bind:value={addStepName} placeholder="name *"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
        <input class="add-input" bind:value={addStepCmd} placeholder="command *"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') submitAddStep($commandCenter.definition) }} />
        <label class="add-checkbox"><input type="checkbox" bind:checked={addStepDefault} /> on by default</label>
        <label class="add-checkbox"><input type="checkbox" bind:checked={addStepDestructive} /> destructive (confirm before enabling)</label>
      </div>
      <div class="add-footer">
        <button class="add-btn-cancel" onclick={() => addStepFor = null}>Cancel</button>
        <button class="add-btn-submit" onclick={() => submitAddStep($commandCenter.definition)}>Add</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 12px;
    height: 32px;
    flex-shrink: 0;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
  }
  .header-label {
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .header-right { display: flex; align-items: center; gap: 4px; margin-left: auto; }

  .hdr-btn, .row-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    padding: 3px 4px;
    border-radius: 3px;
    transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover, .row-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .list { overflow-y: auto; flex: 1; padding: 6px 0; }
  .empty { padding: 10px 12px; color: var(--muted); font-size: 11px; font-style: italic; }

  .card {
    margin: 0 8px 12px;
    padding: 8px 10px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-raised);
  }
  .card-header { display: flex; align-items: center; gap: 6px; }
  .repo-name { font-size: 12px; font-weight: 600; color: var(--foreground); }
  .dep-badge {
    font-family: "SF Mono", Menlo, monospace;
    font-size: 10px; color: var(--muted);
    background: var(--bg-hover); padding: 1px 5px; border-radius: 3px;
  }
  .card-actions { display: flex; align-items: center; gap: 2px; margin-left: auto; }

  .checkout-row { display: flex; align-items: center; gap: 6px; margin: 8px 0 4px; }

  .select-wrap { position: relative; display: inline-flex; align-items: center; }
  select {
    appearance: none;
    -webkit-appearance: none;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 11px;
    padding: 4px 24px 4px 8px;
    outline: none;
    cursor: pointer;
    transition: border-color 0.1s, background 0.1s;
  }
  select:hover { background: var(--bg-hover); }
  select:focus { border-color: var(--accent); }
  option { background: var(--background); color: var(--foreground); }
  .select-wrap :global(.select-chevron) {
    position: absolute;
    right: 7px;
    color: var(--muted);
    pointer-events: none;
  }

  .branch-input {
    flex: 1;
    background: var(--background);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 11px;
    padding: 4px 8px;
    outline: none;
    font-family: "SF Mono", Menlo, monospace;
  }
  .branch-input:focus { border-color: var(--accent); }

  .section-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase;
    color: var(--muted); padding: 8px 2px 3px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 2px;
    font-size: 12px;
  }
  .svc-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .status-dot {
    width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0;
    background: var(--muted); position: relative;
  }
  .status-dot.running { background: var(--success); }
  .status-dot.crashed { background: var(--error); }
  .status-dot.stopped { background: var(--muted); }
  .status-dot.running::after {
    content: '';
    position: absolute;
    inset: -4px;
    border-radius: 50%;
    background: var(--success);
    opacity: 0;
    animation: ring-pulse 2.4s ease-out infinite;
  }
  @keyframes ring-pulse {
    0%   { transform: scale(0.6); opacity: 0.5; }
    80%  { transform: scale(2.0); opacity: 0; }
    100% { transform: scale(2.0); opacity: 0; }
  }

  .badge {
    font-family: "SF Mono", Menlo, monospace;
    font-size: 10px; padding: 1px 5px; border-radius: 3px;
    background: var(--bg-hover); color: var(--muted);
    border: none;
  }
  .badge.danger { color: var(--error); }
  .badge.port {
    display: flex; align-items: center; gap: 2px;
    color: color-mix(in srgb, var(--accent) 80%, var(--foreground));
    cursor: pointer; transition: background 0.1s;
  }
  .badge.port:hover { background: var(--bg-selected); }

  .add-link {
    display: flex; align-items: center; gap: 3px;
    background: none; border: none; color: var(--muted);
    font-size: 10px; cursor: pointer; padding: 3px 2px; margin-top: 2px;
  }
  .add-link:hover { color: var(--accent); }

  .chip-row { display: flex; flex-wrap: wrap; gap: 4px; padding: 2px; }
  .chip {
    background: var(--bg-hover); border: 1px solid var(--border); border-radius: 10px;
    color: var(--muted); font-size: 10px; padding: 2px 8px; cursor: pointer;
  }
  .chip.active { background: var(--accent); border-color: var(--accent); color: #000; }

  .env-box {
    width: 100%;
    background: var(--background); border: 1px solid var(--border);
    border-radius: 5px; color: var(--foreground); font-size: 11px;
    padding: 6px 8px; outline: none; resize: vertical;
    font-family: "SF Mono", Menlo, monospace;
    box-sizing: border-box;
  }
  .env-box:focus { border-color: var(--accent); }

  /* ── add service / add step dialogs ── */
  .add-overlay {
    position: fixed; inset: 0; z-index: 9000;
    background: rgba(0,0,0,0.45);
    display: flex; align-items: center; justify-content: center;
  }
  .add-panel {
    width: 340px; background: var(--bg-raised);
    border: 1px solid var(--border); border-radius: 10px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.5);
    display: flex; flex-direction: column; overflow: hidden;
  }
  .add-header {
    display: flex; align-items: center;
    padding: 10px 14px 8px; border-bottom: 1px solid var(--border);
  }
  .add-title { font-size: 12px; font-weight: 600; color: var(--muted); flex: 1; }
  .add-close {
    background: none; border: none; color: var(--muted);
    cursor: pointer; font-size: 13px; padding: 2px 5px; border-radius: 3px;
  }
  .add-close:hover { color: var(--foreground); background: var(--bg-hover); }
  .add-body { padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; }
  .add-input {
    background: var(--background); border: 1px solid var(--border);
    border-radius: 5px; color: var(--foreground); font-size: 12px;
    padding: 6px 8px; outline: none;
    font-family: "SF Mono", Menlo, monospace;
  }
  .add-input:focus { border-color: var(--accent); }
  .add-checkbox { display: flex; align-items: center; gap: 6px; font-size: 11px; color: var(--muted); }
  .add-footer {
    display: flex; justify-content: flex-end; gap: 8px;
    padding: 8px 14px 12px; border-top: 1px solid var(--border);
  }
  .add-btn-cancel {
    background: none; border: 1px solid var(--border); border-radius: 5px;
    color: var(--muted); font-size: 11px; padding: 5px 12px; cursor: pointer;
  }
  .add-btn-cancel:hover { color: var(--foreground); background: var(--bg-hover); }
  .add-btn-submit {
    background: var(--accent); border: none; border-radius: 5px;
    color: #000; font-size: 11px; font-weight: 600; padding: 5px 14px; cursor: pointer;
  }
  .add-btn-submit:hover { opacity: 0.85; }
</style>
