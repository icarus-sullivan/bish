<script lang="ts">
  import { commandCenter, projectRoot, openLogsTab, openPreviewTab } from '../lib/stores'
  import {
    GetCommandCenterBranches, SetCommandCenterTarget, SaveCommandCenterDefinition,
    StartCommandCenterRepo, StopCommandCenterRepo, StartCommandCenterService,
    StartAllCommandCenter, StopAllCommandCenter, RefreshCommandCenterRepo,
    DetectRepoEnv, ApplyRepoProposal, CreateEnv, SetActiveEnv, DestroyEnv, ClearStepCache,
    DefaultCommandCenterDBSpec,
  } from '../lib/wails'
  import type { CCRepo, CCTarget, CCStep, CCBranchInfo, CCDefinition, CCService, CCServiceStatus, CCProposal, CCDBSpec } from '../lib/wails'
  import { IconPlus, IconPlayerPlayFilled, IconPlayerStopFilled, IconRefresh, IconTrash, IconChevronDown, IconExternalLink, IconListDetails, IconGripVertical, IconWand, IconPencil, IconAlertTriangle } from '@tabler/icons-svelte'
  import { modalA11y } from '../lib/a11y'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { features } from '../lib/features'
  import { toast } from '../lib/toast'

  // harness sub-flags all require the master switch (mirrors
  // commandcenter.Manager.FeatureOn); preview stands alone — it's inert
  const harness = $derived(!!$features.harness)
  const fDetect = $derived(harness && !!$features.harnessDetect)
  const fEnvs = $derived(harness && !!$features.harnessEnvs)
  const fDb = $derived(harness && !!$features.harnessDb)
  const fCache = $derived(harness && !!$features.harnessCache)
  const fHealth = $derived(harness && !!$features.harnessHealth)
  const fPreview = $derived(!!$features.harnessPreview)

  const activeEnv = $derived($commandCenter.state.envs.find(e => e.name === $commandCenter.state.active))
  const portOffset = $derived(activeEnv?.portOffset ?? 0)
  const drift = $derived($commandCenter.drift[$commandCenter.state.active] ?? [])

  let branchesCache: Record<string, CCBranchInfo[]> = $state({})
  let envDraft: Record<string, string> = $state({})

  // add/edit service dialog — editService is the original name when editing
  let addServiceFor: string | null = $state(null)
  let editService: string | null = $state(null)
  let addServiceName = $state('')
  let addServiceCmd = $state('')
  let addServicePort = $state('')
  let addServicePortEnv = $state('')
  let addServicePortArgs = $state('')
  let addHealthHttp = $state('')
  let addHealthLog = $state('')
  let addHealthCmd = $state('')
  let addHealthTimeout = $state('')

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

  // switching env swaps every card's targets — reseed the env drafts
  let draftEnv = ''
  $effect(() => {
    const active = $commandCenter.state.active
    if (active === draftEnv) return
    draftEnv = active
    for (const r of $commandCenter.definition.repos) {
      envDraft[r.id] = envToText($commandCenter.state.targets[r.id]?.env)
    }
  })

  // auto-open Preview on the not-ready → ready edge (own toggle, default off)
  const wasReady = new Map<string, boolean>()
  $effect(() => {
    const auto = harness && !!$features.harnessPreviewAuto && fPreview
    for (const [key, st] of Object.entries($commandCenter.statuses)) {
      const ready = st.ready && st.status === 'running'
      if (auto && ready && wasReady.get(key) === false && st.port > 0) {
        openPreviewTab(`http://localhost:${st.port}`, `${key.split('|')[1]} :${st.port}`)
      }
      wasReady.set(key, ready)
    }
  })

  async function run(p: Promise<unknown>) {
    try { await p } catch (e) { toast.error(String(e)) }
  }

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

  function saveRepo(def: CCDefinition, repo: CCRepo, patch: Partial<CCRepo>) {
    return run(SaveCommandCenterDefinition({ repos: def.repos.map(r => r.id === repo.id ? { ...r, ...patch } : r) }))
  }

  function openAddService(repoId: string, svc?: CCService) {
    addServiceFor = repoId
    editService = svc?.name ?? null
    addServiceName = svc?.name ?? ''
    addServiceCmd = svc?.cmd ?? ''
    addServicePort = svc?.port ? String(svc.port) : ''
    addServicePortEnv = svc?.portEnv ?? ''
    addServicePortArgs = svc?.portArgs ?? ''
    addHealthHttp = svc?.health?.http ?? ''
    addHealthLog = svc?.health?.logMatch ?? ''
    addHealthCmd = svc?.health?.cmd ?? ''
    addHealthTimeout = svc?.health?.timeoutSec ? String(svc.health.timeoutSec) : ''
  }
  async function submitAddService(def: CCDefinition) {
    if (!addServiceFor || !addServiceName.trim() || !addServiceCmd.trim()) return
    const port = parseInt(addServicePort, 10) || 0
    const health = {
      http: addHealthHttp.trim() || undefined,
      logMatch: addHealthLog.trim() || undefined,
      cmd: addHealthCmd.trim() || undefined,
      timeoutSec: parseInt(addHealthTimeout, 10) || undefined,
    }
    const hasHealth = Object.values(health).some(v => v !== undefined)
    const svc: CCService = {
      name: addServiceName.trim(), cmd: addServiceCmd.trim(), port,
      portEnv: addServicePortEnv.trim() || undefined,
      portArgs: addServicePortArgs.trim() || undefined,
      health: hasHealth ? health : undefined,
    }
    const editing = editService
    const repos = def.repos.map(r => {
      if (r.id !== addServiceFor) return r
      const list = r.services ?? []
      return { ...r, services: editing ? list.map(s => s.name === editing ? { ...s, ...svc } : s) : [...list, svc] }
    })
    await run(SaveCommandCenterDefinition({ repos }))
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

  // ── step cache: only install/codegen, never destructive/stateful ──
  const CACHE_INPUTS = ['package.json', '**/package.json', 'pnpm-lock.yaml', 'yarn.lock', 'package-lock.json', 'bun.lockb',
    'go.sum', 'Cargo.lock', 'poetry.lock', 'uv.lock', 'requirements.txt', 'Gemfile.lock']
  function cacheKind(step: CCStep): string {
    if (step.destructive || step.stateful) return ''
    if (step.kind) return step.kind === 'install' || step.kind === 'codegen' ? step.kind : ''
    return /\binstall\b/.test(step.cmd) ? 'install' : ''
  }
  function toggleStepCache(def: CCDefinition, repo: CCRepo, step: CCStep) {
    const on = !!step.cacheInputs?.length
    const kind = cacheKind(step)
    saveRepo(def, repo, {
      steps: (repo.steps ?? []).map(s => s.name === step.name ? { ...s, kind, cacheInputs: on ? [] : CACHE_INPUTS } : s),
    })
  }
  async function startRepo(e: MouseEvent, repo: CCRepo) {
    // ⇧-click: ignore the step cache for this start
    if (e.shiftKey && fCache) await run(ClearStepCache(repo.id))
    await run(StartCommandCenterRepo(repo.id))
  }

  // ── status / ports ──
  function statusFor(repoId: string, key: string): CCServiceStatus | undefined {
    return $commandCenter.statuses[repoId + '|' + key]
  }
  function dotClass(st: CCServiceStatus | undefined) {
    if (!st) return ''
    if (st.status === 'crashed' || st.phase === 'unhealthy') return 'crashed'
    if (st.phase === 'starting' || st.phase === 'waiting-deps') return 'pending'
    if (st.status === 'running') return 'running'
    return 'stopped'
  }
  function dotTitle(st: CCServiceStatus | undefined) {
    if (!st) return 'not running'
    const phase = st.phase || st.status
    return st.detail ? `${phase} — ${st.detail}` : phase
  }

  function openPort(e: MouseEvent, svcName: string, port: number) {
    e.stopPropagation()
    const url = `http://localhost:${port}`
    if (fPreview) openPreviewTab(url, `${svcName} :${port}`)
    else BrowserOpenURL(url)
  }
  function openExternal(e: MouseEvent, port: number) {
    e.stopPropagation()
    BrowserOpenURL(`http://localhost:${port}`)
  }

  function ports(svc: { port: number }, st: CCServiceStatus | undefined) {
    if (st?.ports?.length) return st.ports
    if (st?.port) return [st.port]
    return svc.port ? [svc.port + portOffset] : []
  }

  function viewLogs(repo: CCRepo, name: string, st: { processId?: string } | undefined) {
    if (!st?.processId) return
    openLogsTab(st.processId, repo.name + ' · ' + name)
  }

  // ── detect services ──
  let proposal = $state<CCProposal | null>(null)
  let proposalRepo = $state('')
  let detecting = $state('')
  async function detect(repo: CCRepo) {
    detecting = repo.id
    try {
      const p = await DetectRepoEnv(repo.id)
      proposal = { ...p, services: p.services ?? [], steps: p.steps ?? [], notes: p.notes ?? [] }
      proposalRepo = repo.id
    } catch (e) {
      toast.error(String(e))
    } finally {
      detecting = ''
    }
  }
  async function applyProposal() {
    if (!proposal) return
    await run(ApplyRepoProposal(proposalRepo, proposal))
    proposal = null
  }
  function existsIn(repoId: string, kind: 'services' | 'steps', name: string) {
    const r = $commandCenter.definition.repos.find(x => x.id === repoId)
    return !!(r?.[kind] ?? []).find(x => x.name === name)
  }

  // ── environments ──
  let newEnvOpen = $state(false)
  let newEnvName = $state('')
  let newEnvBranch = $state('')
  let creatingEnv = $state(false)
  let destroyOpen = $state(false)
  let destroyDropDB = $state(false)
  let destroyWorktrees = $state(false)
  const allBranches = $derived([...new Set(Object.values(branchesCache).flat().map(b => b.name))])

  function onEnvSelect(e: Event) {
    const sel = e.target as HTMLSelectElement
    if (sel.value === '__new__') {
      sel.value = $commandCenter.state.active
      openNewEnv()
      return
    }
    run(SetActiveEnv(sel.value))
  }
  function openNewEnv() {
    newEnvName = ''; newEnvBranch = ''
    newEnvOpen = true
  }
  async function submitNewEnv() {
    if (!newEnvName.trim() || !newEnvBranch.trim() || creatingEnv) return
    creatingEnv = true
    try {
      await CreateEnv(newEnvName.trim(), newEnvBranch.trim())
      newEnvOpen = false
    } catch (e) {
      toast.error(String(e))
    } finally {
      creatingEnv = false
    }
  }
  function openDestroy() {
    destroyDropDB = false; destroyWorktrees = false
    destroyOpen = true
  }
  async function submitDestroy() {
    const name = $commandCenter.state.active
    if (destroyDropDB && !confirm(`Drop database ${activeEnv?.dbName}? Its data is gone for good.`)) return
    destroyOpen = false
    await run(DestroyEnv(name, destroyDropDB, destroyWorktrees))
  }

  // ── database ──
  async function setDBMode(def: CCDefinition, repo: CCRepo, mode: string) {
    if (!mode) { saveRepo(def, repo, { db: null }); return }
    const spec = await DefaultCommandCenterDBSpec(mode).catch(() => null)
    if (spec) saveRepo(def, repo, { db: spec })
  }
  function setDBField(def: CCDefinition, repo: CCRepo, field: keyof CCDBSpec, value: string) {
    if (!repo.db) return
    const v: string | number = field === 'port' ? (parseInt(value, 10) || 0) : value
    if (repo.db[field] === v) return
    saveRepo(def, repo, { db: { ...repo.db, [field]: v } as CCDBSpec })
  }
  const DB_FIELDS: { key: keyof CCDBSpec; label: string; modes?: string[] }[] = [
    { key: 'template', label: 'template', modes: ['template'] },
    { key: 'file', label: 'file', modes: ['compose'] },
    { key: 'port', label: 'port' },
    { key: 'create', label: 'create' },
    { key: 'drop', label: 'drop' },
    { key: 'ready', label: 'ready' },
    { key: 'urlEnv', label: 'url env' },
    { key: 'url', label: 'url' },
  ]

  // drag-to-reorder services (list order = start order)
  let dragSvc = $state<{ repoId: string; name: string } | null>(null)
  let dropBeforeSvc = $state<string | null>(null)

  function onSvcDragStart(e: DragEvent, repoId: string, name: string) {
    dragSvc = { repoId, name }
    e.dataTransfer!.effectAllowed = 'move'
  }
  function onSvcDragOver(e: DragEvent, repoId: string, name: string) {
    if (!dragSvc || dragSvc.repoId !== repoId) return
    e.preventDefault()
    e.dataTransfer!.dropEffect = 'move'
    dropBeforeSvc = name
  }
  function onSvcDrop(e: DragEvent, def: CCDefinition, repo: CCRepo) {
    e.preventDefault()
    if (dragSvc && dragSvc.repoId === repo.id) reorderServices(def, repo, dragSvc.name, dropBeforeSvc)
    dragSvc = null
    dropBeforeSvc = null
  }
  function onSvcDragEnd() {
    dragSvc = null
    dropBeforeSvc = null
  }
  function reorderServices(def: CCDefinition, repo: CCRepo, srcName: string, beforeName: string | null) {
    const services = [...(repo.services ?? [])]
    const srcIdx = services.findIndex(s => s.name === srcName)
    if (srcIdx === -1) return
    const [moved] = services.splice(srcIdx, 1)
    const destIdx = beforeName ? services.findIndex(s => s.name === beforeName) : services.length
    services.splice(destIdx === -1 ? services.length : destIdx, 0, moved)
    SaveCommandCenterDefinition({ repos: def.repos.map(r => r.id === repo.id ? { ...r, services } : r) })
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Command Center</span>
    <div class="header-right">
      {#if fEnvs && $projectRoot}
        <span class="select-wrap env-select">
          <select value={$commandCenter.state.active} onchange={onEnvSelect} title="Environment">
            {#each $commandCenter.state.envs as env (env.name)}
              <option value={env.name}>{env.name}{env.portOffset ? ` +${env.portOffset}` : ''}{$commandCenter.running[env.name] ? ' ●' : ''}</option>
            {/each}
            <option value="__new__">New env…</option>
          </select>
          <IconChevronDown size={13} class="select-chevron" />
        </span>
        <button class="hdr-btn" onclick={openNewEnv} title="New environment"><IconPlus size={13} /></button>
        {#if $commandCenter.state.active !== 'default'}
          <button class="hdr-btn" onclick={openDestroy} title="Destroy this environment"><IconTrash size={13} /></button>
        {/if}
      {/if}
      <button class="hdr-btn" onclick={() => run(StartAllCommandCenter())} title="Start all"><IconPlayerPlayFilled size={13} /></button>
      <button class="hdr-btn" onclick={() => StopAllCommandCenter()} title="Stop all"><IconPlayerStopFilled size={13} /></button>
    </div>
  </div>
  <div class="list">
    {#if fEnvs && activeEnv && activeEnv.name !== 'default'}
      <div class="env-info">
        <span>branch <b>{activeEnv.branch}</b></span>
        <span>ports +{activeEnv.portOffset}</span>
        {#if fDb && activeEnv.dbName}<span>db <b>{activeEnv.dbName}</b></span>{/if}
      </div>
    {/if}
    {#if drift.length}
      <div class="drift">
        {#each drift as w}<div><IconAlertTriangle size={11} /> {w}</div>{/each}
      </div>
    {/if}
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
                {#if fDetect}
                  <button class="hdr-btn" disabled={detecting === repo.id} onclick={() => detect(repo)} title="Detect services"><IconWand size={13} /></button>
                {/if}
                <button class="hdr-btn" onclick={() => RefreshCommandCenterRepo(repo.id)} title={`Fetch ${repo.mainBranch}`}><IconRefresh size={13} /></button>
                <button class="hdr-btn" onclick={(e) => startRepo(e, repo)} title={fCache ? `Start ${repo.name} (⇧-click: ignore step cache)` : `Start ${repo.name}`}><IconPlayerPlayFilled size={13} /></button>
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
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div class="row" role="listitem"
                  class:dragging={dragSvc?.repoId === repo.id && dragSvc?.name === svc.name}
                  class:drop-before={dragSvc !== null && dragSvc.repoId === repo.id && dragSvc.name !== svc.name && dropBeforeSvc === svc.name}
                  draggable="true"
                  ondragstart={(e) => onSvcDragStart(e, repo.id, svc.name)}
                  ondragover={(e) => onSvcDragOver(e, repo.id, svc.name)}
                  ondrop={(e) => onSvcDrop(e, $commandCenter.definition, repo)}
                  ondragend={onSvcDragEnd}>
                  <IconGripVertical class="grip" size={11} />
                  <input type="checkbox" checked={(target.services ?? []).includes(svc.name)} onchange={() => toggleService(repo, target, svc.name)} />
                  <span class="status-dot {dotClass(st)}" title={dotTitle(st)}></span>
                  <span class="svc-name" title={st?.detail ? `${svc.cmd}\n${st.detail}` : svc.cmd}>{svc.name}</span>
                  {#each ports(svc, st) as port (port)}
                    <button class="badge port" onclick={(e) => openPort(e, svc.name, port)} title={fPreview ? `Preview http://localhost:${port}` : `Open http://localhost:${port} in browser`}>
                      {#if !fPreview}<IconExternalLink size={9} />{/if}:{port}
                    </button>
                    {#if fPreview}
                      <button class="row-btn" onclick={(e) => openExternal(e, port)} title="Open http://localhost:{port} in browser"><IconExternalLink size={12} /></button>
                    {/if}
                  {/each}
                  <button class="row-btn" disabled={!st?.processId} onclick={() => viewLogs(repo, svc.name, st)} title="View output"><IconListDetails size={12} /></button>
                  <button class="row-btn" onclick={() => StartCommandCenterService(repo.id, svc.name)} title="Start"><IconPlayerPlayFilled size={12} /></button>
                  <button class="row-btn" onclick={() => openAddService(repo.id, svc)} title="Edit"><IconPencil size={12} /></button>
                  <button class="row-btn" onclick={() => removeService($commandCenter.definition, repo, svc.name)} title="Remove"><IconTrash size={12} /></button>
                </div>
              {/each}
              {#if dragSvc?.repoId === repo.id}
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div class="svc-drop-end" class:drop-before={dropBeforeSvc === null}
                  ondragover={(e) => { e.preventDefault(); e.dataTransfer!.dropEffect = 'move'; dropBeforeSvc = null }}
                  ondrop={(e) => onSvcDrop(e, $commandCenter.definition, repo)}></div>
              {/if}
            {/if}
            {#if !repo.services?.length && fDetect}
              <div class="hint">no services yet — <button class="link" onclick={() => detect(repo)}>detect from repo files</button></div>
            {/if}
            <button class="add-link" onclick={() => openAddService(repo.id)}><IconPlus size={11} /> add service</button>

            {#if repo.steps?.length}
              <div class="section-label">Before start</div>
              {#each repo.steps as step (step.name)}
                <div class="row">
                  <input type="checkbox" checked={stepEnabled(target, step)} onchange={() => toggleStep(repo, target, step)} />
                  <span class="svc-name" title={step.cmd}>{step.name}</span>
                  {#if step.destructive}<span class="badge danger">destructive</span>{/if}
                  {#if fCache && cacheKind(step)}
                    <button class="chip small" class:active={!!step.cacheInputs?.length} onclick={() => toggleStepCache($commandCenter.definition, repo, step)}
                      title={step.cacheInputs?.length ? `Skipped when unchanged: ${step.cacheInputs.join(', ')}` : 'Skip this step when its lockfiles are unchanged since the last successful run'}>cache</button>
                  {/if}
                  <button class="row-btn" onclick={() => removeStep($commandCenter.definition, repo, step.name)} title="Remove"><IconTrash size={12} /></button>
                </div>
              {/each}
            {/if}
            {#if harness && repo.compose}
              <div class="row">
                <span class="svc-name" title="docker compose -f {repo.compose} up -d — once per project, shared by every env">infra · {repo.compose}</span>
                <button class="row-btn" onclick={() => saveRepo($commandCenter.definition, repo, { compose: '' })} title="Remove"><IconTrash size={12} /></button>
              </div>
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

            {#if fDb}
              <div class="section-label">Database (per env)</div>
              <div class="checkout-row">
                <span class="select-wrap">
                  <select value={repo.db?.mode ?? ''} onchange={(e) => setDBMode($commandCenter.definition, repo, (e.target as HTMLSelectElement).value)}>
                    <option value="">none</option>
                    <option value="template">template (clone on shared server)</option>
                    <option value="compose">compose (own stack per env)</option>
                  </select>
                  <IconChevronDown size={13} class="select-chevron" />
                </span>
              </div>
              {#if repo.db}
                <div class="db-grid">
                  {#each DB_FIELDS.filter(f => !f.modes || f.modes.includes(repo.db?.mode ?? '')) as f (f.key)}
                    <span class="db-label">{f.label}</span>
                    <input class="branch-input" value={repo.db[f.key] ?? ''} spellcheck="false"
                      onchange={(e) => setDBField($commandCenter.definition, repo, f.key, (e.target as HTMLInputElement).value)} />
                  {/each}
                </div>
              {/if}
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
        <span class="add-title">{editService ? 'Edit Service' : 'Add Service'}</span>
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
        {#if fEnvs}
          <div class="add-sub">How it takes a different port (needed for envs with an offset)</div>
          <input class="add-input" bind:value={addServicePortEnv} placeholder="port env var, e.g. PORT"
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
          <input class="add-input" bind:value={addServicePortArgs} placeholder={'port args, e.g. --port {{port}}'}
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
        {/if}
        {#if fHealth}
          <div class="add-sub">Health — ready when every filled check passes (empty = port accepts connections)</div>
          <input class="add-input" bind:value={addHealthHttp} placeholder="http path, e.g. /health"
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
          <input class="add-input" bind:value={addHealthLog} placeholder="log regexp, e.g. ready in \d+ms"
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
          <input class="add-input" bind:value={addHealthCmd} placeholder="command, exit 0 = ready"
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
          <input class="add-input" bind:value={addHealthTimeout} placeholder="timeout seconds (default 600)"
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
        {/if}
      </div>
      <div class="add-footer">
        <button class="add-btn-cancel" onclick={() => addServiceFor = null}>Cancel</button>
        <button class="add-btn-submit" onclick={() => submitAddService($commandCenter.definition)}>{editService ? 'Save' : 'Add'}</button>
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

{#if proposal}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="add-overlay" onclick={() => proposal = null}>
    <div class="add-panel wide" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={() => proposal = null} onclick={(e) => e.stopPropagation()}>
      <div class="add-header">
        <span class="add-title">Detected in {proposalRepo}{proposal.toolchain ? ` · ${proposal.toolchain}` : ''}</span>
        <button class="add-close" onclick={() => proposal = null} aria-label="Close">✕</button>
      </div>
      <div class="add-body proposal">
        {#if !proposal.services.length && !proposal.steps.length}
          <div class="hint">Nothing recognizable in this repo's files.</div>
        {/if}
        {#each [{ label: 'Services', list: proposal.services, kind: 'services' as const }, { label: 'Before start', list: proposal.steps, kind: 'steps' as const }] as group (group.label)}
          {#if group.list.length}
            <div class="section-label">{group.label}</div>
            {#each group.list as c (c.name)}
              {@const have = c.kind !== 'infra' && existsIn(proposalRepo, group.kind, c.name)}
              <label class="cand" class:muted={have}>
                <input type="checkbox" bind:checked={c.accept} disabled={have} />
                <span class="cand-main">
                  <span class="cand-line">
                    <b>{c.name}</b>
                    <code>{c.cmd}</code>
                    {#if c.port}<span class="badge">:{c.port}</span>{/if}
                    {#if c.kind !== 'service'}<span class="badge">{c.kind}</span>{/if}
                    {#if c.destructive}<span class="badge danger">destructive</span>{/if}
                    {#if have}<span class="badge">already defined</span>{/if}
                  </span>
                  <span class="cand-src">from {c.source}{c.supersedes?.length ? ` · covers ${c.supersedes.join(', ')}` : ''}{c.portArgs ? ` · port via ${c.portArgs}` : c.portEnv ? ` · port via $${c.portEnv}` : ''} · {c.confidence}%</span>
                </span>
              </label>
            {/each}
          {/if}
        {/each}
        {#if proposal.notes.length}
          <div class="section-label">Notes</div>
          {#each proposal.notes as n}<div class="cand-src">{n}</div>{/each}
        {/if}
      </div>
      <div class="add-footer">
        <button class="add-btn-cancel" onclick={() => proposal = null}>Cancel</button>
        <button class="add-btn-submit" onclick={applyProposal}>Apply selected</button>
      </div>
    </div>
  </div>
{/if}

{#if newEnvOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="add-overlay" onclick={() => newEnvOpen = false}>
    <div class="add-panel" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={() => newEnvOpen = false} onclick={(e) => e.stopPropagation()}>
      <div class="add-header">
        <span class="add-title">New Environment</span>
        <button class="add-close" onclick={() => newEnvOpen = false} aria-label="Close">✕</button>
      </div>
      <div class="add-body">
        <input class="add-input" bind:value={newEnvName} placeholder="name * (e.g. review-42)"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false" />
        <input class="add-input" bind:value={newEnvBranch} placeholder="branch *" list="cc-env-branches"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') submitNewEnv() }} />
        <datalist id="cc-env-branches">
          {#each allBranches as b}<option value={b}></option>{/each}
        </datalist>
        <div class="add-sub">Every repo running in “{$commandCenter.state.active}” gets a worktree on this branch, the same ticked services, and its own port offset{fDb ? ' and database' : ''}.</div>
      </div>
      <div class="add-footer">
        <button class="add-btn-cancel" onclick={() => newEnvOpen = false}>Cancel</button>
        <button class="add-btn-submit" disabled={creatingEnv} onclick={submitNewEnv}>{creatingEnv ? 'Creating…' : 'Create'}</button>
      </div>
    </div>
  </div>
{/if}

{#if destroyOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="add-overlay" onclick={() => destroyOpen = false}>
    <div class="add-panel" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={() => destroyOpen = false} onclick={(e) => e.stopPropagation()}>
      <div class="add-header">
        <span class="add-title">Destroy “{$commandCenter.state.active}”</span>
        <button class="add-close" onclick={() => destroyOpen = false} aria-label="Close">✕</button>
      </div>
      <div class="add-body">
        <div class="add-sub">Stops its services and forgets the env. Other envs keep running.</div>
        {#if fDb && activeEnv?.dbName}
          <label class="add-checkbox"><input type="checkbox" bind:checked={destroyDropDB} /> drop database {activeEnv.dbName} (unticked = kept for a fast restart)</label>
        {/if}
        <label class="add-checkbox"><input type="checkbox" bind:checked={destroyWorktrees} /> remove its worktrees (fails safely if they have uncommitted changes)</label>
      </div>
      <div class="add-footer">
        <button class="add-btn-cancel" onclick={() => destroyOpen = false}>Cancel</button>
        <button class="add-btn-submit danger" onclick={submitDestroy}>Destroy</button>
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
  .row-btn:disabled { opacity: 0.3; cursor: default; }
  .row-btn:disabled:hover { color: var(--muted); background: none; }

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

  .row :global(.grip) { color: var(--muted); flex-shrink: 0; cursor: grab; opacity: 0.5; }
  .row:hover :global(.grip) { opacity: 1; }
  .row.dragging { opacity: 0.35; }
  .row.drop-before { position: relative; }
  .row.drop-before::before {
    content: '';
    position: absolute;
    left: 0; right: 0; top: -2px;
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
  }
  .svc-drop-end {
    height: 6px;
    margin: -2px 0 2px;
    position: relative;
  }
  .svc-drop-end.drop-before::before {
    content: '';
    position: absolute;
    left: 2px; right: 2px; top: 2px;
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
  }

  .status-dot {
    width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0;
    background: var(--muted); position: relative;
  }
  .status-dot.running { background: var(--success); }
  .status-dot.crashed { background: var(--error); }
  .status-dot.stopped { background: var(--muted); }
  .status-dot.pending { background: var(--warning); animation: pending-blink 1.2s ease-in-out infinite; }
  @keyframes pending-blink {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
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
  .chip.small { font-size: 9px; padding: 1px 6px; }

  .hdr-btn:disabled { opacity: 0.4; cursor: default; }
  .env-select select { max-width: 130px; padding: 2px 22px 2px 6px; font-size: 10px; }
  .env-info {
    display: flex; flex-wrap: wrap; gap: 10px;
    margin: 0 8px 8px; padding: 4px 8px;
    font-size: 10px; color: var(--muted);
    border: 1px dashed var(--border); border-radius: 5px;
  }
  .env-info b { color: var(--foreground); font-weight: 600; }
  .drift {
    margin: 0 8px 8px; padding: 6px 8px; border-radius: 5px;
    font-size: 10px; line-height: 1.5;
    color: var(--warning); background: color-mix(in srgb, var(--warning) 10%, transparent);
    border: 1px solid color-mix(in srgb, var(--warning) 35%, transparent);
  }
  .drift div { display: flex; align-items: center; gap: 4px; }
  .hint { font-size: 10px; color: var(--muted); padding: 2px; }
  .link { background: none; border: none; color: var(--accent); cursor: pointer; font-size: 10px; padding: 0; }
  .db-grid {
    display: grid; grid-template-columns: auto 1fr; gap: 4px 6px; align-items: center;
    margin: 4px 0;
  }
  .db-label { font-size: 10px; color: var(--muted); }

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
  .add-sub { font-size: 10px; color: var(--muted); margin-top: 4px; line-height: 1.4; }
  .add-panel.wide { width: 560px; max-height: 80vh; }
  .add-body.proposal { overflow-y: auto; gap: 2px; }
  .cand { display: flex; align-items: flex-start; gap: 8px; padding: 4px 2px; cursor: pointer; }
  .cand.muted { opacity: 0.5; cursor: default; }
  .cand input { margin-top: 2px; }
  .cand-main { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .cand-line { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; font-size: 12px; }
  .cand-line code { font-family: "SF Mono", Menlo, monospace; font-size: 11px; color: var(--muted); }
  .cand-src { font-size: 10px; color: var(--muted); font-family: "SF Mono", Menlo, monospace; }
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
  .add-btn-submit:disabled { opacity: 0.5; cursor: default; }
  .add-btn-submit.danger { background: var(--error); color: #fff; }
</style>
