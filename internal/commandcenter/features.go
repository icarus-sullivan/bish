package commandcenter

// Harness feature ids, mirrored from frontend/src/lib/features.ts. Every
// sub-flag also requires the "harness" master, and all of them default off
// (missing key = off) — OFF means the code path never runs, not hidden UI.
const (
	FeatHarness = "harness"
	FeatDetect  = "harnessDetect"
	FeatEnvs    = "harnessEnvs"
	FeatDB      = "harnessDb"
	FeatCache   = "harnessCache"
	FeatHealth  = "harnessHealth"
)

// SetFeatures replaces the feature-toggle map (config.Config.Features).
func (m *Manager) SetFeatures(f map[string]bool) {
	cp := make(map[string]bool, len(f))
	for k, v := range f {
		cp[k] = v
	}
	m.mu.Lock()
	m.features = cp
	m.mu.Unlock()
}

// FeatureOn reports whether a harness feature is enabled: the master switch
// plus, for a sub-flag, the sub-flag itself.
func (m *Manager) FeatureOn(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.featureOnLocked(id)
}

func (m *Manager) featureOnLocked(id string) bool {
	if !m.features[FeatHarness] {
		return false
	}
	return id == FeatHarness || m.features[id]
}
