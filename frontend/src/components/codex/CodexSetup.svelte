<script lang="ts">
  // Gate in front of the Codex panel: installs bish's managed Codex
  // automatically when none exists, then makes sure the user is signed in
  // (ChatGPT in the browser, or an API key) — no terminal steps.
  import type { Snippet } from 'svelte'
  import { IconLoader2, IconBrandOpenai, IconKey } from '@tabler/icons-svelte'
  import { on, CodexInstalled, CodexEnsureInstalled, CodexCall } from '../../lib/wails'
  import { projectRoot, cwd } from '../../lib/stores'
  import { browserHandle } from '../../lib/claude/providers'
  import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'

  let { children }: { children: Snippet } = $props()

  type Phase = 'checking' | 'installing' | 'install-failed' | 'auth-check' | 'signed-out' | 'signing-in' | 'ready'
  let phase = $state<Phase>('checking')
  let error = $state('')
  let progress = $state({ done: 0, total: 0 })
  let apiKeyMode = $state(false)
  let apiKey = $state('')
  let loginId = ''
  let offLogin: (() => void) | null = null

  const root = $derived($projectRoot || $cwd)
  const pct = $derived(progress.total ? Math.round((progress.done / progress.total) * 100) : 0)
  const mb = (n: number) => (n / (1 << 20)).toFixed(0)

  async function install() {
    error = ''
    phase = 'checking'
    if (!(await CodexInstalled().catch(() => false))) {
      phase = 'installing'
      progress = { done: 0, total: 0 }
      const off = on('codex:install', (p: { done: number; total: number }) => { progress = p })
      try {
        await CodexEnsureInstalled()
      } catch (e) {
        error = String(e)
        phase = 'install-failed'
        return
      } finally {
        off()
      }
    }
    await checkAuth()
  }

  async function checkAuth() {
    phase = 'auth-check'
    try {
      const h = await browserHandle(root)
      const r = JSON.parse(await CodexCall(h, 'account/read', '{}'))
      phase = r?.account || r?.requiresOpenaiAuth === false ? 'ready' : 'signed-out'
    } catch (e) {
      // can't tell (older Codex without account/read) — let the chat surface any auth error
      phase = 'ready'
    }
  }

  async function signIn(type: 'chatgpt' | 'apiKey') {
    error = ''
    try {
      const h = await browserHandle(root)
      offLogin?.()
      offLogin = on(`codex:msg:${h}`, (raw: string) => {
        let m: any
        try { m = JSON.parse(raw) } catch { return }
        if (m.method === 'account/login/completed') {
          offLogin?.(); offLogin = null
          if (m.params?.success) checkAuth()
          else { error = m.params?.error || 'Sign-in failed.'; phase = 'signed-out' }
        } else if (m.method === 'account/updated') {
          checkAuth()
        }
      })
      phase = 'signing-in'
      const params = type === 'apiKey' ? { type, apiKey: apiKey.trim() } : { type }
      const r = JSON.parse(await CodexCall(h, 'account/login/start', JSON.stringify(params)))
      if (r?.type === 'chatgpt' && r.authUrl) {
        loginId = r.loginId
        BrowserOpenURL(r.authUrl)
      } else {
        apiKey = ''
        await checkAuth()
      }
    } catch (e) {
      error = String(e)
      phase = 'signed-out'
    }
  }

  async function cancelSignIn() {
    try {
      const h = await browserHandle(root)
      if (loginId) await CodexCall(h, 'account/login/cancel', JSON.stringify({ loginId }))
    } catch {}
    offLogin?.(); offLogin = null
    phase = 'signed-out'
  }

  let started = false
  $effect(() => {
    if (!root || started) return
    started = true
    install()
  })
  $effect(() => () => offLogin?.())
</script>

{#if phase === 'ready'}
  {@render children()}
{:else}
  <div class="setup">
    <div class="glyph">❯_</div>
    {#if phase === 'checking' || phase === 'auth-check'}
      <div class="t"><IconLoader2 size={14} class="spin" /> Starting Codex…</div>
    {:else if phase === 'installing'}
      <div class="t">Setting up Codex</div>
      <div class="d">One-time download of the official Codex build{progress.total ? ` (${mb(progress.total)} MB)` : ''}.</div>
      <div class="bar"><div class="fill" style:width={pct + '%'}></div></div>
      <div class="d small">{progress.total ? `${mb(progress.done)} / ${mb(progress.total)} MB` : 'Connecting…'}</div>
    {:else if phase === 'install-failed'}
      <div class="t">Couldn't set up Codex</div>
      <div class="err">{error}</div>
      <button class="primary" onclick={install}>Try again</button>
    {:else if phase === 'signed-out' || phase === 'signing-in'}
      <div class="t">Sign in to Codex</div>
      <div class="d">Use your ChatGPT plan, or an OpenAI API key.</div>
      {#if phase === 'signing-in'}
        <div class="d"><IconLoader2 size={13} class="spin" /> Finish signing in in your browser…</div>
        <button onclick={cancelSignIn}>Cancel</button>
      {:else if apiKeyMode}
        <input type="password" placeholder="sk-…" bind:value={apiKey} autocomplete="off" spellcheck="false"
               onkeydown={(e) => { if (e.key === 'Enter' && apiKey.trim()) signIn('apiKey') }} />
        <div class="row">
          <button class="primary" disabled={!apiKey.trim()} onclick={() => signIn('apiKey')}>Save key</button>
          <button onclick={() => { apiKeyMode = false; apiKey = '' }}>Back</button>
        </div>
      {:else}
        <button class="primary wide" onclick={() => signIn('chatgpt')}><IconBrandOpenai size={14} /> Sign in with ChatGPT</button>
        <button class="wide" onclick={() => apiKeyMode = true}><IconKey size={14} /> Use an API key</button>
      {/if}
      {#if error}<div class="err">{error}</div>{/if}
    {/if}
  </div>
{/if}

<style>
  .setup { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 48px 20px; text-align: center; }
  .glyph { font-family: "SF Mono", Menlo, monospace; font-size: 24px; color: var(--accent); }
  .t { display: flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 600; color: var(--foreground); }
  .d { display: flex; align-items: center; gap: 6px; font-size: 11.5px; color: var(--muted); }
  .d.small { font-size: 10.5px; }
  .bar { width: 220px; height: 4px; border-radius: 2px; background: var(--bg-raised); overflow: hidden; }
  .fill { height: 100%; background: var(--accent); transition: width 0.15s; }
  .err {
    max-width: 320px; font-size: 11px; color: var(--error); white-space: pre-wrap;
    font-family: "SF Mono", Menlo, monospace;
  }
  .row { display: flex; gap: 6px; }
  button {
    display: flex; align-items: center; justify-content: center; gap: 6px;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 5px; color: var(--foreground);
    font-size: 12px; padding: 6px 12px; cursor: pointer; font-family: inherit;
  }
  button:hover:not(:disabled) { border-color: var(--accent); }
  button:disabled { opacity: 0.45; cursor: default; }
  button.primary { border-color: var(--accent); }
  button.wide { width: 230px; }
  input {
    width: 230px; background: var(--background); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 12px; padding: 6px 9px; outline: none; font-family: "SF Mono", Menlo, monospace;
  }
  input:focus { border-color: var(--accent); }
</style>
