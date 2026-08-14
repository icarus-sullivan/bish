<script lang="ts">
  import {
    debugState, debugOutput, startDebugging, continueExec,
    stepOver, stepIn, stepOut, stopDebugging,
  } from '../lib/dap'
  import type { DebugFrame } from '../lib/dap'
  import { openFileTab, pendingGoto } from '../lib/stores'
  import {
    IconBug, IconPlayerPlayFilled, IconPlayerStopFilled,
    IconPlayerTrackNextFilled, IconStepInto, IconStepOut,
  } from '@tabler/icons-svelte'

  const paused = $derived($debugState.status === 'paused')
  const active = $derived($debugState.status === 'running' || $debugState.status === 'paused' || $debugState.status === 'starting')
  const idle = $derived(!active)

  function jump(f: DebugFrame) {
    if (!f.path) return
    pendingGoto.set({ path: f.path, line: f.line, col: 0 })
    openFileTab(f.path, true)
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Debug</span>
    <div class="toolbar">
      {#if idle}
        <button class="hdr-btn" onclick={startDebugging} title="Start Debugging"><IconBug size={13} /></button>
      {:else}
        <button class="hdr-btn" disabled={!paused} onclick={continueExec} title="Continue"><IconPlayerPlayFilled size={13} /></button>
        <button class="hdr-btn" disabled={!paused} onclick={stepOver} title="Step Over"><IconPlayerTrackNextFilled size={13} /></button>
        <button class="hdr-btn" disabled={!paused} onclick={stepIn} title="Step Into"><IconStepInto size={13} /></button>
        <button class="hdr-btn" disabled={!paused} onclick={stepOut} title="Step Out"><IconStepOut size={13} /></button>
        <button class="hdr-btn" onclick={stopDebugging} title="Stop"><IconPlayerStopFilled size={13} /></button>
      {/if}
    </div>
  </div>

  <div class="status" class:paused class:error={$debugState.status === 'error'}>
    {$debugState.status}{#if $debugState.error}: {$debugState.error}{/if}
  </div>

  <div class="section-label">Call Stack</div>
  <div class="list frames">
    {#if $debugState.frames.length === 0}
      <div class="empty">Not paused</div>
    {:else}
      {#each $debugState.frames as f (f.id)}
        <button class="row" onclick={() => jump(f)}>
          <span class="fname">{f.name}</span>
          <span class="floc">{f.path.split('/').pop()}:{f.line}</span>
        </button>
      {/each}
    {/if}
  </div>

  <div class="section-label">Variables</div>
  <div class="list vars">
    {#if $debugState.variables.length === 0}
      <div class="empty">No variables</div>
    {:else}
      {#each $debugState.variables as v (v.name)}
        <div class="row var-row">
          <span class="vname">{v.name}</span>
          <span class="vval" title={v.value}>{v.value}</span>
        </div>
      {/each}
    {/if}
  </div>

  <div class="section-label">Output</div>
  <div class="list output">
    {#each $debugOutput as line, i (i)}
      <div class="out-line">{line}</div>
    {/each}
  </div>
</div>

<style>
  .panel { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
  .header {
    display: flex; align-items: center; padding: 0 12px; height: 32px;
    flex-shrink: 0; background: var(--bg-raised); border-bottom: 1px solid var(--border);
  }
  .header-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.1em;
    text-transform: uppercase; color: var(--muted);
  }
  .toolbar { display: flex; align-items: center; gap: 1px; margin-left: auto; }
  .hdr-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .hdr-btn:disabled { opacity: 0.35; cursor: default; }
  .hdr-btn:disabled:hover { background: none; }

  .status {
    padding: 5px 12px; font-size: 11px; color: var(--muted);
    text-transform: capitalize; border-bottom: 1px solid var(--border);
  }
  .status.paused { color: var(--warning); }
  .status.error { color: var(--error); }

  .section-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase;
    color: var(--muted); padding: 6px 12px 2px;
  }
  .list { overflow-y: auto; flex-shrink: 0; max-height: 22%; padding: 2px 0; }
  .list.output { flex: 1; max-height: none; font-family: "SF Mono", Menlo, monospace; font-size: 11px; padding: 2px 12px; }
  .empty { color: var(--muted); font-size: 12px; padding: 4px 12px; }

  .row {
    display: flex; align-items: baseline; gap: 8px; width: 100%;
    padding: 3px 12px; background: none; border: none; cursor: pointer;
    text-align: left; color: var(--foreground); font-size: 12px;
  }
  button.row:hover { background: var(--bg-hover); }
  .fname { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .floc { color: var(--muted); font-size: 10px; flex-shrink: 0; }

  .var-row { cursor: default; font-family: "SF Mono", Menlo, monospace; font-size: 11px; }
  .vname { color: var(--accent); flex-shrink: 0; }
  .vval { color: var(--foreground); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .out-line { white-space: pre-wrap; word-break: break-all; color: var(--muted); line-height: 1.5; }
</style>
