// Cmd+K inline edit: a one-shot, ephemeral assistant session scoped to a
// single selection — separate from AssistantPanel's persistent chat session
// so a quick inline edit doesn't clutter unrelated conversation history.
// 'plan' permission mode is a safety net: even if the model reaches for
// Edit/Write instead of just answering in text, plan mode blocks it from
// touching disk directly — we apply the accepted text ourselves as a normal
// editor transaction instead.
import { get } from 'svelte/store'
import { AssistantStart, AssistantSend, AssistantStop, on } from './wails'
import { projectRoot, cwd } from './stores'

function extractCodeBlock(text: string): string | null {
  const m = text.match(/```[^\n]*\n([\s\S]*?)```/)
  return m ? m[1].replace(/\n$/, '') : null
}

export function requestInlineEdit(path: string, selectedText: string, instruction: string): Promise<string> {
  const root = get(projectRoot) || get(cwd)
  return new Promise<string>((resolve, reject) => {
    let buf = ''
    let settled = false
    let offMsg: (() => void) | null = null

    function cleanup(id: string) {
      if (settled) return
      settled = true
      offMsg?.()
      AssistantStop(id).catch(() => {})
    }

    AssistantStart(root, 'plan').then(id => {
      offMsg = on(`assistant:msg:${id}`, (raw: string) => {
        let msg: any
        try { msg = JSON.parse(raw) } catch { return }
        if (msg.type === 'assistant') {
          for (const block of msg.message?.content ?? []) {
            if (block.type === 'text' && block.text) buf += block.text
          }
        } else if (msg.type === 'result') {
          cleanup(id)
          if (msg.is_error) { reject(new Error(msg.result ?? 'Assistant error')); return }
          const code = extractCodeBlock(buf)
          if (code == null) {
            reject(new Error('No direct edit returned — try rephrasing, or use the Assistant panel for multi-step changes.'))
            return
          }
          resolve(code)
        }
      })
      const prompt = [
        `You are editing a snippet from ${path}. Reply with ONLY a single fenced code block containing ` +
        `the complete replacement for the selected code below — no explanation, no markdown outside the ` +
        `fence, and do not use any tools.`,
        '',
        `Instruction: ${instruction}`,
        '',
        'Selected code:',
        '```',
        selectedText,
        '```',
      ].join('\n')
      return AssistantSend(id, prompt).catch(err => { cleanup(id); reject(err) })
    }).catch(reject)
  })
}
