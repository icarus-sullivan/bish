// Well-known Claude Code CLI slash commands. Informational/insertable only —
// the spawned `claude` subprocess interprets these itself once sent.
export interface SlashCommand { name: string; description: string; terminalOnly?: boolean }

export const SLASH_COMMANDS: SlashCommand[] = [
  { name: '/clear', description: 'Clear conversation history' },
  { name: '/compact', description: 'Summarize history to free up context' },
  { name: '/cost', description: 'Show token usage and cost for this session' },
  { name: '/help', description: 'Show available commands' },
  { name: '/init', description: 'Generate a CLAUDE.md for this project' },
  { name: '/permissions', description: 'View or change tool permissions' },
  { name: '/review', description: 'Review a pull request' },
  { name: '/model', description: 'Switch the active model' },
  { name: '/agents', description: 'Manage custom subagents' },
  { name: '/memory', description: 'Edit memory files' },
  { name: '/status', description: 'Show session status and config' },
  { name: '/bug', description: 'Report a bug to Anthropic' },
  // These need an interactive TTY (submenu UI / OAuth browser flow) that the
  // embedded assistant's headless subprocess can't provide — run in the Terminal panel instead.
  { name: '/mcp', description: 'Manage MCP servers — run in the Terminal panel', terminalOnly: true },
  { name: '/plugin', description: 'Manage plugins — run in the Terminal panel', terminalOnly: true },
  { name: '/login', description: 'Sign in — run in the Terminal panel', terminalOnly: true },
]
