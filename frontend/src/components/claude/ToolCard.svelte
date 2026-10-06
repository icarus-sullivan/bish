<script lang="ts">
  import {
    IconChevronRight, IconChevronDown, IconCheck, IconX, IconLoader2, IconFileText, IconTerminal2, IconPencil,
    IconSearch, IconWorld, IconRobot, IconListCheck, IconPlug, IconTool, IconCircle, IconCircleCheck,
    IconCircleDot, IconExternalLink, IconGitCompare, IconMap, IconHelpCircle, IconAlertTriangle,
  } from '@tabler/icons-svelte'
  import type { ToolItem } from '../../lib/claude/conversation.svelte'
  import type { AgentConversation } from '../../lib/claude/agent'
  import {
    toolSummary, toolLabel, mcpParts, relPath, renderMarkdown, describeSuggestion, FILE_EDIT_TOOLS,
  } from '../../lib/claude/format'
  import { lineDiff, diffStats } from '../../lib/claude/diff'
  import { openFileTab, openDiffTab, pendingGoto } from '../../lib/stores'
  import DiffBlock from './DiffBlock.svelte'
  import ToolCard from './ToolCard.svelte'

  let { t, conv, root, nested = false }: { t: ToolItem; conv: AgentConversation; root: string; nested?: boolean } = $props()

  const isEdit = $derived(FILE_EDIT_TOOLS.has(t.name))
  const isPatch = $derived(t.name === 'apply_patch')
  const patchChanges = $derived<any[]>(isPatch && Array.isArray(t.input?.changes) ? t.input.changes : [])
  const patchStats = $derived.by(() => {
    let add = 0, del = 0
    for (const c of patchChanges) for (const l of String(c.diff ?? '').split('\n')) {
      if (l.startsWith('+') && !l.startsWith('+++')) add++
      else if (l.startsWith('-') && !l.startsWith('---')) del++
    }
    return { add, del }
  })
  const isPlan = $derived(t.name === 'ExitPlanMode')
  const isQuestion = $derived(t.name === 'AskUserQuestion')
  const isTodo = $derived(t.name === 'TodoWrite')
  const isAgent = $derived(t.name === 'Task' || t.name === 'Agent')
  const pending = $derived(t.ask?.state === 'pending')
  const filePath = $derived<string>(t.input?.file_path ?? t.input?.notebook_path ?? '')

  // edits and anything awaiting approval open by default; the rest collapsed
  let userOpen = $state<boolean | null>(null)
  const open = $derived(userOpen ?? (pending || ((isEdit || isPatch) && !nested) || isPlan || isQuestion || isTodo))

  const summary = $derived(toolSummary(t.name, t.input, root))

  const edits = $derived.by(() => {
    if (!isEdit) return []
    const i = t.input ?? {}
    if (t.name === 'Edit') return [{ old: i.old_string ?? '', new: i.new_string ?? '' }]
    if (t.name === 'MultiEdit') return (i.edits ?? []).map((e: any) => ({ old: e.old_string ?? '', new: e.new_string ?? '' }))
    if (t.name === 'Write' || t.name === 'write_file') return [{ old: '', new: i.content ?? '' }]
    if (t.name === 'NotebookEdit') return [{ old: '', new: i.new_source ?? '' }]
    return []
  })
  const stats = $derived.by(() => {
    let add = 0, del = 0
    for (const e of edits) { const s = diffStats(lineDiff(e.old, e.new)); add += s.add; del += s.del }
    return { add, del }
  })

  function openFile(p: string, line = 0) {
    if (!p) return
    const abs = p.startsWith('/') ? p : root + '/' + p
    openFileTab(abs)
    if (line > 0) pendingGoto.set({ path: abs, line, col: 0 })
  }

  // ─── permission answers ───────────────────────────────────────────────────
  let denyNote = $state('')
  let showDeny = $state(false)

  function allow(suggestionIdx: number[] = []) { conv.respond(t, true, { suggestionIdx }) }
  function deny() { conv.respond(t, false, { message: denyNote.trim() || undefined }) }
  function denyAndStop() { conv.respond(t, false, { message: 'The user stopped this.', interrupt: true }) }

  // plan: approve and pick how edits get approved from here on
  let planFeedback = $state('')
  async function approvePlan(nextMode: 'acceptEdits' | 'manual') {
    await conv.respond(t, true)
    await conv.setMode(nextMode)
  }
  function keepPlanning() {
    conv.respond(t, false, { message: planFeedback.trim() || 'The user wants to keep planning. Revise the plan.' })
  }

  // AskUserQuestion: per-question selected labels + free-text "Other"
  let picks = $state<Record<number, string[]>>({})
  let other = $state<Record<number, string>>({})
  const questions = $derived<any[]>(Array.isArray(t.input?.questions) ? t.input.questions : [])
  function toggle(qi: number, label: string, multi: boolean) {
    const cur = picks[qi] ?? []
    picks[qi] = multi ? (cur.includes(label) ? cur.filter(l => l !== label) : [...cur, label]) : [label]
    if (!multi) other[qi] = ''
  }
  const answerReady = $derived(questions.every((_, qi) => (picks[qi]?.length ?? 0) > 0 || (other[qi] ?? '').trim()))
  function submitAnswers() {
    const answers: Record<string, string> = {}
    questions.forEach((q, qi) => {
      const parts = [...(picks[qi] ?? [])]
      if ((other[qi] ?? '').trim()) parts.push(other[qi].trim())
      answers[q.question] = parts.join(', ')
    })
    conv.respond(t, true, { updatedInput: { ...$state.snapshot(t.input), answers } })
  }

  function iconFor(name: string) {
    if (mcpParts(name)) return IconPlug
    switch (name) {
      case 'Read': return IconFileText
      case 'Bash': case 'BashOutput': case 'KillShell': case 'KillBash': return IconTerminal2
      case 'Write': case 'Edit': case 'MultiEdit': case 'NotebookEdit': case 'apply_patch': return IconPencil
      case 'Grep': case 'Glob': case 'ToolSearch': return IconSearch
      case 'WebFetch': case 'WebSearch': return IconWorld
      case 'Task': case 'Agent': return IconRobot
      case 'TodoWrite': return IconListCheck
      case 'ExitPlanMode': return IconMap
      case 'AskUserQuestion': return IconHelpCircle
      default: return IconTool
    }
  }
  const Icon = $derived(iconFor(t.name))
  // number keys answer the ask, like the terminal prompt — only when focus
  // isn't in a text field, and only for the most recent pending card
  $effect(() => {
    if (!pending || isQuestion) return
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null
      if (el?.closest?.('input, textarea, [contenteditable], .cm-editor, .xterm')) return
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (conv.pendingAsks[conv.pendingAsks.length - 1]?.id !== t.id) return
      const n = Number(e.key)
      if (!n) return
      if (isPlan) {
        if (n === 1) approvePlan('acceptEdits')
        else if (n === 2) approvePlan('manual')
        else return
      } else if (n === 1) allow()
      else if (!t.ask!.suppressAlways && n - 2 < Math.min(3, t.ask!.suggestions.length)) allow([n - 2])
      else return
      e.preventDefault()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const resultLines = $derived(t.result?.text ? t.result.text.split('\n') : [])
  let showAllOutput = $state(false)
  const OUTPUT_LINES = 40
</script>

{#if isPlan}
  <div class="card plan" class:pending>
    <div class="card-head static"><IconMap size={13} /> <span class="label">Plan</span>
      {#if t.ask && t.ask.state !== 'pending'}
        <span class="state {t.ask.state}">{t.ask.state === 'allowed' ? 'Approved' : t.ask.state === 'denied' ? 'Kept planning' : 'Cancelled'}</span>
      {/if}
    </div>
    <div class="md plan-body">{@html renderMarkdown(t.input?.plan ?? '')}</div>
    {#if pending}
      <div class="ask">
        <div class="ask-q">Ready to code?</div>
        <div class="ask-actions col">
          <button class="opt primary" onclick={() => approvePlan('acceptEdits')}><span class="k">1</span> Yes, and auto-accept edits</button>
          <button class="opt" onclick={() => approvePlan('manual')}><span class="k">2</span> Yes, and manually approve edits</button>
          <div class="deny-row">
            <input class="note" placeholder="No, keep planning — tell Claude what to change…" bind:value={planFeedback}
                   onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); keepPlanning() } }} />
            <button class="opt" onclick={keepPlanning}>Keep planning</button>
          </div>
        </div>
      </div>
    {:else if !t.ask && t.status !== 'done'}
      <div class="waiting"><IconLoader2 size={12} class="spin" /> Waiting for approval request…</div>
    {/if}
  </div>
{:else if isQuestion}
  <div class="card question" class:pending>
    <div class="card-head static"><IconHelpCircle size={13} /> <span class="label">Claude has {questions.length > 1 ? 'questions' : 'a question'}</span>
      {#if t.ask && t.ask.state !== 'pending'}<span class="state {t.ask.state}">{t.ask.state === 'allowed' ? 'Answered' : t.ask.state === 'denied' ? 'Declined' : 'Cancelled'}</span>{/if}
    </div>
    {#each questions as q, qi (qi)}
      <div class="q">
        {#if q.header}<span class="q-chip">{q.header}</span>{/if}
        <div class="q-text">{q.question}</div>
        <div class="q-opts">
          {#each q.options ?? [] as o (o.label)}
            <button class="q-opt" class:sel={(picks[qi] ?? []).includes(o.label)} disabled={!pending}
                    onclick={() => toggle(qi, o.label, !!q.multiSelect)}>
              <span class="q-mark">{q.multiSelect ? ((picks[qi] ?? []).includes(o.label) ? '☑' : '☐') : ((picks[qi] ?? []).includes(o.label) ? '◉' : '○')}</span>
              <span class="q-body"><span class="q-label">{o.label}</span>{#if o.description}<span class="q-desc">{o.description}</span>{/if}</span>
            </button>
            {#if o.preview && (picks[qi] ?? []).includes(o.label)}<pre class="q-preview">{o.preview}</pre>{/if}
          {/each}
          {#if pending}
            <input class="note" placeholder="Other…" bind:value={other[qi]}
                   oninput={() => { if (!q.multiSelect && other[qi]) picks[qi] = [] }} />
          {/if}
        </div>
      </div>
    {/each}
    {#if pending}
      <div class="ask-actions">
        <button class="opt primary" disabled={!answerReady} onclick={submitAnswers}><IconCheck size={12} /> Submit</button>
        <button class="opt" onclick={() => conv.respond(t, false, { message: 'The user declined to answer.' })}>Skip</button>
      </div>
    {:else if t.result}
      <div class="q-result">{t.result.text}</div>
    {/if}
  </div>
{:else if isTodo && !nested}
  <div class="card todo">
    <div class="card-head static"><IconListCheck size={13} /> <span class="label">Todos</span><span class="sum">{summary}</span></div>
    <ul class="todos">
      {#each t.input?.todos ?? [] as td, i (i)}
        <li class={td.status}>
          {#if td.status === 'completed'}<IconCircleCheck size={13} />{:else if td.status === 'in_progress'}<IconCircleDot size={13} />{:else}<IconCircle size={13} />{/if}
          <span>{td.status === 'in_progress' && td.activeForm ? td.activeForm : td.content}</span>
        </li>
      {/each}
    </ul>
  </div>
{:else}
  <div class="card" class:pending class:nested class:err={t.status === 'error'} class:denied={t.status === 'denied'}>
    <button class="card-head" onclick={() => userOpen = !open}>
      {#if open}<IconChevronDown size={12} />{:else}<IconChevronRight size={12} />{/if}
      <Icon size={13} />
      <span class="label">{toolLabel(t.name)}</span>
      {#if isAgent && t.input?.subagent_type}<span class="chip">{t.input.subagent_type}</span>{/if}
      <span class="sum" title={summary}>{summary}</span>
      {#if isEdit && (stats.add || stats.del)}
        <span class="stat"><span class="a">+{stats.add}</span> <span class="d">−{stats.del}</span></span>
      {:else if isPatch && (patchStats.add || patchStats.del)}
        <span class="stat"><span class="a">+{patchStats.add}</span> <span class="d">−{patchStats.del}</span></span>
      {/if}
      <span class="status-icon">
        {#if t.status === 'streaming' || (t.status === 'running' && !pending)}<IconLoader2 size={12} class="spin" />
        {:else if t.status === 'error'}<IconX size={12} />
        {:else if t.status === 'denied'}<IconX size={12} />
        {:else if t.status === 'done'}<IconCheck size={12} />{/if}
      </span>
    </button>

    {#if open}
      <div class="card-body">
        {#if isEdit}
          <div class="file-row">
            <button class="link" onclick={() => openFile(filePath)} title={filePath}>{relPath(filePath, root)}</button>
            <button class="mini" title="Open file" onclick={() => openFile(filePath)}><IconExternalLink size={12} /></button>
            {#if t.status === 'done'}<button class="mini" title="Git diff vs HEAD" onclick={() => openDiffTab(filePath)}><IconGitCompare size={12} /></button>{/if}
          </div>
          {#each edits as e, i (i)}
            <DiffBlock oldText={e.old} newText={e.new} />
          {/each}
          {#if t.status === 'streaming' && t.partialJson}
            <div class="streaming-note">Writing… {Math.round(t.partialJson.length / 1024)} KB</div>
          {/if}
        {:else if isPatch}
          {#each patchChanges as c, i (i)}
            <div class="file-row">
              <span class="chip">{c.kind?.type ?? 'update'}</span>
              <button class="link" onclick={() => openFile(c.path)} title={c.path}>{relPath(c.path, root)}</button>
              {#if c.kind?.move_path || c.kind?.movePath}<span class="hint">→ {relPath(c.kind.move_path ?? c.kind.movePath, root)}</span>{/if}
              {#if t.status === 'done'}<button class="mini" title="Git diff vs HEAD" onclick={() => openDiffTab(c.path)}><IconGitCompare size={12} /></button>{/if}
            </div>
            {#if c.diff}<DiffBlock unified={c.diff} unifiedKind={c.kind?.type ?? 'update'} />{/if}
          {/each}
        {:else if t.name === 'Bash'}
          <pre class="cmd"><span class="prompt">$</span> {t.input?.command ?? ''}</pre>
          {#if t.input?.run_in_background}<div class="hint">runs in background</div>{/if}
        {:else if t.name === 'Read'}
          <div class="file-row">
            <button class="link" onclick={() => openFile(filePath, t.input?.offset ?? 0)}>{relPath(filePath, root)}</button>
          </div>
        {:else if isAgent}
          {#if t.input?.prompt}<details class="prompt"><summary>Prompt</summary><pre>{t.input.prompt}</pre></details>{/if}
          {#if t.children.length}
            <div class="children">
              {#each t.children as c (c.id)}<ToolCard t={c} {conv} {root} nested />{/each}
            </div>
          {/if}
          {#if t.childText && t.status === 'running'}<div class="hint">{t.childText}</div>{/if}
        {:else if t.status === 'streaming'}
          <pre class="json">{t.partialJson}</pre>
        {:else}
          <pre class="json">{JSON.stringify(t.input, null, 2)}</pre>
        {/if}

        {#if t.result && !((isEdit || isPatch) && !t.result.isError) && t.name !== 'Read'}
          <div class="output" class:error={t.result.isError}>
            {#if isAgent && !t.result.isError}
              <div class="md">{@html renderMarkdown(t.result.text)}</div>
            {:else}
              <pre>{(showAllOutput ? resultLines : resultLines.slice(0, OUTPUT_LINES)).join('\n')}</pre>
              {#if resultLines.length > OUTPUT_LINES && !showAllOutput}
                <button class="more" onclick={() => showAllOutput = true}>Show all {resultLines.length} lines</button>
              {/if}
            {/if}
          </div>
        {:else if t.result?.isError}
          <div class="output error"><pre>{t.result.text}</pre></div>
        {:else if t.result && t.name === 'Read'}
          <div class="hint">{resultLines.length} lines read</div>
        {/if}
      </div>
    {/if}

    {#if t.ask}
      {#if pending}
        <div class="ask">
          {#if t.ask.reason}<div class="reason"><IconAlertTriangle size={12} /> {t.ask.reason}</div>{/if}
          <div class="ask-q">{t.ask.title || `Allow Claude to use ${toolLabel(t.name)}?`}</div>
          {#if t.ask.blockedPath}<div class="hint">Outside the allowed directories: <code>{t.ask.blockedPath}</code></div>{/if}
          <div class="ask-actions col">
            <!-- svelte-ignore a11y_autofocus -->
            <button class="opt primary" autofocus={!t.ask.defaultToNo} onclick={() => allow()}><span class="k">1</span> Yes</button>
            {#if !t.ask.suppressAlways}
              {#each t.ask.suggestions.slice(0, 3) as s, si (si)}
                <button class="opt" onclick={() => allow([si])}><span class="k">{si + 2}</span> Yes — {describeSuggestion(s)}</button>
              {/each}
            {/if}
            {#if showDeny}
              <div class="deny-row">
                <!-- svelte-ignore a11y_autofocus -->
                <input class="note" autofocus placeholder="Tell Claude what to do instead…" bind:value={denyNote}
                       onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); deny() } }} />
                <button class="opt danger" onclick={deny}>Deny</button>
              </div>
            {:else}
              <div class="row">
                <button class="opt danger" onclick={() => showDeny = true}>No, and tell Claude what to do differently</button>
                <button class="opt" onclick={denyAndStop} title="Deny and stop the turn">Stop</button>
              </div>
            {/if}
          </div>
        </div>
      {:else if t.ask.state === 'denied'}
        <div class="ask-done denied">Denied{t.ask.note ? ` — “${t.ask.note}”` : ''}</div>
      {:else if t.ask.state === 'cancelled'}
        <div class="ask-done">Request cancelled</div>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .card {
    border: 1px solid var(--border); border-radius: 6px; background: var(--bg-raised);
    overflow: hidden; font-size: 12px; max-width: 100%;
  }
  .card.nested { border-radius: 4px; background: var(--background); }
  .card.pending { border-color: var(--accent); box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 30%, transparent); }
  .card.err { border-color: color-mix(in srgb, var(--error) 60%, var(--border)); }
  .card.denied { opacity: 0.75; }
  .card-head {
    display: flex; align-items: center; gap: 6px; width: 100%; padding: 5px 8px;
    background: none; border: none; color: var(--muted); font-size: 11.5px; text-align: left; cursor: pointer;
    font-family: inherit;
  }
  .card-head.static { cursor: default; color: var(--foreground); }
  button.card-head:hover { color: var(--foreground); }
  .label { font-weight: 600; color: var(--foreground); flex-shrink: 0; }
  .chip {
    font-size: 10px; border: 1px solid var(--border); border-radius: 8px; padding: 0 5px; flex-shrink: 0;
  }
  .sum {
    flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    font-family: "SF Mono", Menlo, monospace; font-size: 11px;
  }
  .stat { font-family: "SF Mono", Menlo, monospace; font-size: 10.5px; flex-shrink: 0; }
  .stat .a { color: var(--success); }
  .stat .d { color: var(--error); }
  .status-icon { display: flex; flex-shrink: 0; }
  .card.err .status-icon, .card.denied .status-icon { color: var(--error); }
  .state { margin-left: auto; font-size: 10.5px; color: var(--muted); font-weight: 500; }
  .state.allowed { color: var(--success); }
  .state.denied { color: var(--warning); }

  .card-body { border-top: 1px solid var(--border); }
  .file-row { display: flex; align-items: center; gap: 4px; padding: 4px 8px; }
  .link {
    background: none; border: none; color: var(--accent); cursor: pointer; padding: 0; font-size: 11px;
    font-family: "SF Mono", Menlo, monospace; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0;
  }
  .link:hover { text-decoration: underline; }
  .mini {
    display: flex; background: none; border: none; color: var(--muted); cursor: pointer; padding: 2px 3px; border-radius: 3px;
  }
  .mini:hover { color: var(--foreground); background: var(--bg-hover); }
  pre {
    margin: 0; padding: 6px 8px; font-family: "SF Mono", Menlo, monospace; font-size: 11px;
    white-space: pre-wrap; word-break: break-word; max-height: 320px; overflow: auto;
  }
  .cmd { background: var(--background); color: var(--foreground); }
  .prompt { color: var(--accent); user-select: none; }
  .json { color: var(--muted); background: var(--background); }
  .output { border-top: 1px solid var(--border); background: var(--background); }
  .output pre { color: var(--foreground); opacity: 0.85; }
  .output.error pre { color: var(--error); opacity: 1; }
  .output .md { padding: 6px 8px; }
  .more {
    display: block; width: 100%; background: var(--bg-raised); border: none; border-top: 1px solid var(--border);
    color: var(--muted); font-size: 11px; padding: 4px; cursor: pointer;
  }
  .hint { font-size: 11px; color: var(--muted); padding: 4px 8px; }
  .hint code { font-size: 10.5px; }
  .streaming-note { font-size: 11px; color: var(--muted); padding: 4px 8px; }
  .children { display: flex; flex-direction: column; gap: 4px; padding: 6px 8px; }
  details.prompt { padding: 4px 8px; font-size: 11px; color: var(--muted); }
  details.prompt summary { cursor: pointer; }

  .ask { border-top: 1px solid var(--border); padding: 8px; display: flex; flex-direction: column; gap: 6px; background: color-mix(in srgb, var(--accent) 6%, var(--bg-raised)); }
  .ask-q { font-weight: 600; color: var(--foreground); }
  .reason { display: flex; gap: 5px; align-items: flex-start; font-size: 11px; color: var(--warning); white-space: pre-wrap; }
  .ask-actions { display: flex; gap: 6px; flex-wrap: wrap; }
  .ask-actions.col { flex-direction: column; align-items: stretch; }
  .row, .deny-row { display: flex; gap: 6px; }
  .opt {
    display: flex; align-items: center; gap: 6px; text-align: left;
    background: var(--background); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 11.5px; padding: 5px 8px; cursor: pointer; font-family: inherit;
    transition: border-color 0.1s, background 0.1s;
  }
  .opt:hover:not(:disabled) { border-color: var(--accent); background: var(--bg-hover); }
  .opt:disabled { opacity: 0.45; cursor: default; }
  .opt.primary { border-color: var(--accent); }
  .opt.danger:hover { border-color: var(--error); }
  .row .opt.danger { flex: 1; }
  .k {
    font-family: "SF Mono", Menlo, monospace; font-size: 10px; color: var(--muted);
    border: 1px solid var(--border); border-radius: 3px; padding: 0 4px;
  }
  .note {
    flex: 1; min-width: 0; background: var(--background); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 11.5px; padding: 5px 8px; outline: none; font-family: inherit;
  }
  .note:focus { border-color: var(--accent); }
  .ask-done { border-top: 1px solid var(--border); padding: 4px 8px; font-size: 11px; color: var(--muted); }
  .ask-done.denied { color: var(--warning); }
  .waiting { display: flex; align-items: center; gap: 5px; padding: 6px 8px; color: var(--muted); font-size: 11px; }

  .plan { border-color: color-mix(in srgb, var(--accent) 60%, var(--border)); }
  .plan-body { padding: 4px 10px 8px; border-top: 1px solid var(--border); }

  .q { padding: 6px 8px; border-top: 1px solid var(--border); display: flex; flex-direction: column; gap: 5px; }
  .q-chip { align-self: flex-start; font-size: 10px; color: var(--accent); border: 1px solid var(--accent); border-radius: 8px; padding: 0 6px; }
  .q-text { font-weight: 600; color: var(--foreground); }
  .q-opts { display: flex; flex-direction: column; gap: 4px; }
  .q-opt {
    display: flex; gap: 8px; align-items: flex-start; text-align: left; font-family: inherit;
    background: var(--background); border: 1px solid var(--border); border-radius: 5px; padding: 5px 8px;
    color: var(--foreground); cursor: pointer; font-size: 11.5px;
  }
  .q-opt:hover:not(:disabled) { border-color: var(--accent); }
  .q-opt.sel { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 12%, var(--background)); }
  .q-opt:disabled { cursor: default; }
  .q-mark { color: var(--accent); flex-shrink: 0; }
  .q-body { display: flex; flex-direction: column; gap: 1px; }
  .q-label { font-weight: 600; }
  .q-desc { color: var(--muted); font-size: 11px; }
  .q-preview { background: var(--background); border: 1px solid var(--border); border-radius: 4px; }
  .q-result { padding: 6px 8px; color: var(--muted); font-size: 11px; border-top: 1px solid var(--border); white-space: pre-wrap; }
  .question .ask-actions { padding: 6px 8px 8px; }

  .todos { list-style: none; margin: 0; padding: 4px 8px 8px; display: flex; flex-direction: column; gap: 3px; border-top: 1px solid var(--border); }
  .todos li { display: flex; gap: 6px; align-items: flex-start; color: var(--foreground); }
  .todos li :global(svg) { flex-shrink: 0; margin-top: 1px; color: var(--muted); }
  .todos li.completed { color: var(--muted); text-decoration: line-through; }
  .todos li.completed :global(svg) { color: var(--success); }
  .todos li.in_progress { font-weight: 600; }
  .todos li.in_progress :global(svg) { color: var(--accent); }

  .md { font-size: 12px; line-height: 1.55; color: var(--foreground); }
  .md :global(p) { margin: 0 0 6px; }
  .md :global(p:last-child) { margin-bottom: 0; }
  .md :global(ul), .md :global(ol) { margin: 0 0 6px; padding-left: 18px; }
  .md :global(h1), .md :global(h2), .md :global(h3), .md :global(h4) { font-size: 12.5px; margin: 8px 0 4px; }
  .md :global(code) { font-family: "SF Mono", Menlo, monospace; font-size: 11px; background: var(--background); padding: 0 3px; border-radius: 3px; }
  .md :global(pre) { background: var(--background); padding: 6px; border-radius: 4px; overflow-x: auto; }
  .md :global(pre code) { padding: 0; }
  :global(.spin) { animation: claude-spin 0.9s linear infinite; }
  @keyframes -global-claude-spin { to { transform: rotate(360deg); } }
</style>
