// Shared modal behavior: focus the first focusable element on open, trap Tab
// inside the dialog, Escape closes, and focus returns to whatever had it
// before the dialog opened. Apply to the dialog panel itself (not the
// backdrop) via `use:modalA11y={onClose}`.
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

export function modalA11y(node: HTMLElement, onClose: () => void) {
  const prevActive = document.activeElement as HTMLElement | null

  function focusables(): HTMLElement[] {
    return Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE))
  }

  ;(focusables()[0] ?? node).focus()

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { e.preventDefault(); onClose(); return }
    if (e.key !== 'Tab') return
    const els = focusables()
    if (els.length === 0) { e.preventDefault(); return }
    const first = els[0], last = els[els.length - 1]
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
  }

  node.addEventListener('keydown', onKeydown)
  return {
    destroy() {
      node.removeEventListener('keydown', onKeydown)
      prevActive?.focus?.()
    },
  }
}
