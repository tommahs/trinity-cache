package mirror

import (
	"fmt"
	"math"
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

// WeightedSelector implements the Selector interface using a scoring algorithm
// that considers effective weight, recent usage, and in-flight downloads.
type WeightedSelector struct {
	mirrors []*Mirror
	mu      sync.RWMutex
	// timeNow is injected for testing time-dependent behavior
	timeNow func() time.Time
}

// NewWeightedSelector creates a new weighted selector.
func NewWeightedSelector() *WeightedSelector {
	return &WeightedSelector{
		mirrors: make([]*Mirror, 0),
		timeNow: time.Now,
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

// Select returns the mirror with the highest score based on effective weight,
// recent usage, and in-flight download count. Returns an error if no mirrors
// are available or none have positive weight.
func (ws *WeightedSelector) Select() (*Mirror, error) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if len(ws.mirrors) == 0 {
		logger.Warn("mirror selection failed", "reason", "no mirrors available")
		return nil, fmt.Errorf("no mirrors available")
	}

	var selected *Mirror
	var maxScore float64
	now := ws.timeNow()

	for _, m := range ws.mirrors {
		score := ws.calculateScore(m, now)
		if score > maxScore {
			maxScore = score
			selected = m
		}
	}

	if selected == nil || maxScore <= 0 {
		logger.Warn("mirror selection failed", "reason", "no mirror with positive weight available")
		return nil, fmt.Errorf("no mirror with positive weight available")
	}

	logger.Debug("mirror selected", "url", selected.URL, "effective_weight", selected.EffectiveWeight, "score", maxScore)
	return selected, nil
}

// calculateScore computes a selection score for a mirror based on:
// - Effective weight (primary factor: higher weight = higher score)
// - Time since last use (secondary: mirrors unused longer get boost)
// - In-flight downloads (tertiary: fewer in-flight downloads = higher score)
//
// The formula is:
//   score = EffectiveWeight * (1 + timeSinceLastUseBoost) / (1 + inFlightPenalty)
//
// This ensures:
// 1. Mirrors with negative or zero effective weight have zero score
// 2. Recently unused mirrors are preferred when weights are similar
// 3. Mirrors with fewer concurrent downloads are preferred
func (ws *WeightedSelector) calculateScore(m *Mirror, now time.Time) float64 {
	// Mirrors with non-positive weight should never be selected
	if m.EffectiveWeight <= 0 {
		return 0
	}

	// Calculate time-since-use boost
	// Mirrors not yet used (zero time) get no boost
	// Mirrors unused for 1 hour get a moderate boost (factor of 2)
	// Mirrors unused for 10+ hours get diminishing returns (log scale)
	var timeSinceLastUseBoost float64
	if !m.LastUsed.IsZero() {
		timeSince := now.Sub(m.LastUsed).Seconds()
		// Boost formula: log(1 + timeSince/3600) where 3600 = 1 hour in seconds
		// This gives a natural diminishing return: 1 hour -> ~0.69, 10 hours -> ~0.89
		timeSinceLastUseBoost = math.Log(1 + timeSince/3600)
	}

	// Calculate in-flight download penalty
	// 0 in-flight downloads = penalty of 0 (no penalty)
	// 1 in-flight download = small penalty (~0.01 reduction)
	// 10 in-flight downloads = moderate penalty (~0.1 reduction)
	inFlightPenalty := float64(m.InFlightDownloads) * 0.01

	// Combine factors: weight * usage-boost / in-flight-penalty
	score := m.EffectiveWeight * (1 + timeSinceLastUseBoost) / (1 + inFlightPenalty)
	return score
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
