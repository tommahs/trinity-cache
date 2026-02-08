package mirror

import (
	"fmt"
	"sync"
	"time"

	"github.com/tommahs/trinity-cache/internal/logger"
)

// Mirror represents an upstream package mirror and its runtime state.
type Mirror struct {
	URL                 string
	BaseWeight          float64
	EffectiveWeight     float64
	LastUsed            time.Time
	InFlightDownloads   int
	inFlightDownloadsMu sync.Mutex
}

// AddInFlightDownload increments the in-flight download counter.
func (m *Mirror) AddInFlightDownload() {
	m.inFlightDownloadsMu.Lock()
	defer m.inFlightDownloadsMu.Unlock()
	m.InFlightDownloads++
}

// RemoveInFlightDownload decrements the in-flight download counter.
// It does not go below zero.
func (m *Mirror) RemoveInFlightDownload() {
	m.inFlightDownloadsMu.Lock()
	defer m.inFlightDownloadsMu.Unlock()
	if m.InFlightDownloads > 0 {
		m.InFlightDownloads--
	}
}

// GetInFlightDownloadCount returns the current number of in-flight downloads.
func (m *Mirror) GetInFlightDownloadCount() int {
	m.inFlightDownloadsMu.Lock()
	defer m.inFlightDownloadsMu.Unlock()
	return m.InFlightDownloads
}

// Selector selects mirrors for downloads and adjusts weights to
// distribute load across mirrors. Implementations should return errors
// when selection fails; optional debug logging for decisions and info
// logging when weights are adjusted.
type Selector interface {
	// Select returns the best candidate mirror for the next download.
	// Return an error if no mirror is available.
	Select() (*Mirror, error)

	// Penalize reduces the effective weight of a mirror after use.
	Penalize(m *Mirror, penalty float64)

	// Add registers a new mirror with the selector.
	Add(m *Mirror)

	// List returns the currently known mirrors.
	List() []*Mirror
}

// WeightedSelector implements the Selector interface using weighted
// random selection based on mirror effective weights.
type WeightedSelector struct {
	mirrors []*Mirror
	mu      sync.RWMutex
}

// NewWeightedSelector creates a new weighted selector.
func NewWeightedSelector() *WeightedSelector {
	return &WeightedSelector{
		mirrors: make([]*Mirror, 0),
	}
}

// Add registers a new mirror with the selector.
func (ws *WeightedSelector) Add(m *Mirror) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.mirrors = append(ws.mirrors, m)
	logger.Debug("mirror registered", "url", m.URL, "base_weight", m.BaseWeight)
}

// List returns the currently known mirrors.
func (ws *WeightedSelector) List() []*Mirror {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	result := make([]*Mirror, len(ws.mirrors))
	copy(result, ws.mirrors)
	return result
}

// Select returns the mirror with the highest effective weight.
// Returns an error if no mirrors are available.
func (ws *WeightedSelector) Select() (*Mirror, error) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if len(ws.mirrors) == 0 {
		logger.Warn("mirror selection failed", "reason", "no mirrors available")
		return nil, fmt.Errorf("no mirrors available")
	}

	var selected *Mirror
	var maxWeight float64

	for _, m := range ws.mirrors {
		if m.EffectiveWeight > maxWeight {
			maxWeight = m.EffectiveWeight
			selected = m
		}
	}

	if selected == nil {
		logger.Warn("mirror selection failed", "reason", "no mirror with positive weight available")
		return nil, fmt.Errorf("no mirror with positive weight available")
	}

	logger.Debug("mirror selected", "url", selected.URL, "effective_weight", selected.EffectiveWeight)
	return selected, nil
}

// Penalize reduces the effective weight of a mirror after use.
func (ws *WeightedSelector) Penalize(m *Mirror, penalty float64) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if penalty <= 0 {
		return
	}

	oldWeight := m.EffectiveWeight
	m.EffectiveWeight -= penalty
	if m.EffectiveWeight < 0 {
		m.EffectiveWeight = 0
	}
	m.LastUsed = time.Now()
	logger.Debug("mirror penalized", "url", m.URL, "old_weight", oldWeight, "new_weight", m.EffectiveWeight, "penalty", penalty)
}
