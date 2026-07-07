# bish — project rules for Claude

## UI: Panel header icon buttons

All small icon buttons that appear in panel headers (Saved Commands, Files, Processes, etc.) must use this exact pattern — no exceptions, no variations:

```css
.hdr-btn {
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
.hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }
```

Use `size={13}` for tabler icons inside these buttons.

Reference implementation: `frontend/src/components/FileTree.svelte` — `.hdr-btn`.

When adding a new header button to any panel, copy `.hdr-btn` verbatim (rename the class if needed, but keep the same property values). Do not add `margin-left`, `border`, `outline`, or background colors.
