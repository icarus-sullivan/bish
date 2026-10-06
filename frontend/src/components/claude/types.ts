// mirrors assistant.SessionSummary (internal/assistant/history.go)
export interface SessionSummary {
  id: string
  title: string
  firstPrompt: string
  gitBranch: string
  modified: number
  size: number
}
