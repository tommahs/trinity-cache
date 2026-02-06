package mirror

import "time"

// Mirror represents an upstream package mirror and its runtime state.
type Mirror struct {
	URL             string
	BaseWeight      float64
	EffectiveWeight float64
	LastUsed        time.Time
}

// Selector selects mirrors for downloads and adjusts weights to
// distribute load across mirrors.
type Selector interface {
	// Select returns the best candidate mirror for the next download.
	Select() (*Mirror, error)

	// Penalize reduces the effective weight of a mirror after use.
	Penalize(m *Mirror, penalty float64)

	// Add registers a new mirror with the selector.
	Add(m *Mirror)

	// List returns the currently known mirrors.
	List() []*Mirror
}
