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

## UI: `<select>` dropdowns

Every `<select>` anywhere in the app must look like the theme picker in Settings — no native browser chrome, no exceptions:

```css
.select-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
}
select {
  appearance: none;
  -webkit-appearance: none;
  background: var(--bg-raised);
  border: 1px solid var(--border);
  border-radius: 5px;
  color: var(--foreground);
  font-size: 12px;
  padding: 6px 28px 6px 10px;
  outline: none;
  cursor: pointer;
  transition: border-color 0.1s, background 0.1s;
}
select:hover { background: var(--bg-hover); }
select:focus { border-color: var(--accent); }
option { background: var(--background); color: var(--foreground); }
.select-wrap :global(.select-chevron) {
  position: absolute;
  right: 9px;
  color: var(--muted);
  pointer-events: none;
}
```
Markup: `<span class="select-wrap"><select>...</select><IconChevronDown size={13} class="select-chevron" /></span>`.

Reference implementation: `frontend/src/components/Settings.svelte` — the Theme select.

Compact placements (e.g. a panel header) may shrink `padding`/`font-size`/drop `min-width`, but must keep `appearance: none` + the absolute-positioned `IconChevronDown` — never fall back to the browser's native select arrow.
