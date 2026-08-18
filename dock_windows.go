package main

import "github.com/csullivan/bish/internal/project"

// setBishDockMenuFromRecents is a no-op on Windows — there's no Dock
// right-click menu to populate.
func setBishDockMenuFromRecents(projects []*project.RecentEntry, files []*project.RecentFile) {}
