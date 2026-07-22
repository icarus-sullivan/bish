// Action command registry — backs the ⌘⇧P command palette. Register a command
// once; it shows up in the palette and can be bound to a key later.
export interface Command {
  id: string
  title: string
  run: () => void
  when?: () => boolean
}

const registry = new Map<string, Command>()

export function registerCommand(cmd: Command): () => void {
  registry.set(cmd.id, cmd)
  return () => registry.delete(cmd.id)
}

export function registerCommands(cmds: Command[]) {
  cmds.forEach(registerCommand)
}

// Commands currently available (respecting each command's `when` guard).
export function listCommands(): Command[] {
  return [...registry.values()].filter(c => !c.when || c.when())
}
