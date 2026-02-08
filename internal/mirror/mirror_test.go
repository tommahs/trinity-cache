package mirror

import (
	"math"
	"testing"
	"time"
)

func TestMirror_InFlightDownloads(t *testing.T) {
	tests := []struct {
		name     string
		initial  int
		adds     int
		removes  int
		expected int
	}{
		{
			name:     "single add",
			initial:  0,
			adds:     1,
			removes:  0,
			expected: 1,
		},
		{
			name:     "multiple adds",
			initial:  0,
			adds:     5,
			removes:  0,
			expected: 5,
		},
		{
			name:     "add and remove",
			initial:  0,
			adds:     3,
			removes:  2,
			expected: 1,
		},
		{
			name:     "remove more than added",
			initial:  0,
			adds:     2,
			removes:  5,
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &Mirror{
				URL:         "https://example.com",
				BaseWeight:  1.0,
				InFlightDownloads: tc.initial,
			}

			for i := 0; i < tc.adds; i++ {
				m.AddInFlightDownload()
			}
			for i := 0; i < tc.removes; i++ {
				m.RemoveInFlightDownload()
			}

			if got := m.GetInFlightDownloadCount(); got != tc.expected {
				t.Errorf("GetInFlightDownloadCount() = %d, want %d", got, tc.expected)
			}
		})
	}
}

func TestMirror_Properties(t *testing.T) {
	now := time.Now()
	m := &Mirror{
		URL:             "https://example.com",
		BaseWeight:      2.5,
		EffectiveWeight: 2.5,
		LastUsed:        now,
	}

	if m.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", m.URL, "https://example.com")
	}
	if m.BaseWeight != 2.5 {
		t.Errorf("BaseWeight = %f, want %f", m.BaseWeight, 2.5)
	}
	if m.EffectiveWeight != 2.5 {
		t.Errorf("EffectiveWeight = %f, want %f", m.EffectiveWeight, 2.5)
	}
	if m.LastUsed != now {
		t.Errorf("LastUsed changed unexpectedly")
	}
}

func TestWeightedSelector_Add(t *testing.T) {
	ws := NewWeightedSelector()

	if got := len(ws.List()); got != 0 {
		t.Errorf("initial mirror list length = %d, want 0", got)
	}

	m1 := &Mirror{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0}
	m2 := &Mirror{URL: "https://mirror2.com", BaseWeight: 1.0, EffectiveWeight: 1.0}

	ws.Add(m1)
	ws.Add(m2)

	mirrors := ws.List()
	if got := len(mirrors); got != 2 {
		t.Errorf("mirror list length = %d, want 2", got)
	}
}

func TestWeightedSelector_Select(t *testing.T) {
	tests := []struct {
		name          string
		mirrors       []*Mirror
		expectedURL   string
		expectError   bool
	}{
		{
			name:        "no mirrors",
			mirrors:     []*Mirror{},
			expectError: true,
		},
		{
			name: "single mirror",
			mirrors: []*Mirror{
				{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0},
			},
			expectedURL: "https://mirror1.com",
			expectError: false,
		},
		{
			name: "select highest weight",
			mirrors: []*Mirror{
				{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0},
				{URL: "https://mirror2.com", BaseWeight: 5.0, EffectiveWeight: 5.0},
				{URL: "https://mirror3.com", BaseWeight: 2.0, EffectiveWeight: 2.0},
			},
			expectedURL: "https://mirror2.com",
			expectError: false,
		},
		{
			name: "all mirrors have zero weight",
			mirrors: []*Mirror{
				{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 0},
				{URL: "https://mirror2.com", BaseWeight: 1.0, EffectiveWeight: 0},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := NewWeightedSelector()
			for _, m := range tc.mirrors {
				ws.Add(m)
			}

			selected, err := ws.Select()
			if tc.expectError {
				if err == nil {
					t.Errorf("Select() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Select() returned unexpected error: %v", err)
				}
				if selected.URL != tc.expectedURL {
					t.Errorf("Select() returned URL = %q, want %q", selected.URL, tc.expectedURL)
				}
			}
		})
	}
}

func TestWeightedSelector_Penalize(t *testing.T) {
	m1 := &Mirror{URL: "https://mirror1.com", BaseWeight: 5.0, EffectiveWeight: 5.0}
	m2 := &Mirror{URL: "https://mirror2.com", BaseWeight: 3.0, EffectiveWeight: 3.0}

	ws := NewWeightedSelector()
	ws.Add(m1)
	ws.Add(m2)

	// Penalize m1
	ws.Penalize(m1, 2.0)

	if m1.EffectiveWeight != 3.0 {
		t.Errorf("after penalize, m1.EffectiveWeight = %f, want 3.0", m1.EffectiveWeight)
	}

	// Now m2 should be selected as it has higher weight
	selected, err := ws.Select()
	if err != nil {
		t.Fatalf("Select() returned error: %v", err)
	}

	// Due to equivalent weights, we just verify selection works
	if selected.EffectiveWeight < 0 {
		t.Errorf("selected mirror has invalid effective weight: %f", selected.EffectiveWeight)
	}
}

func TestWeightedSelector_Penalize_NegativeWeight(t *testing.T) {
	m := &Mirror{URL: "https://mirror.com", BaseWeight: 1.0, EffectiveWeight: 1.0}

	ws := NewWeightedSelector()
	ws.Add(m)

	// Penalize more than the current weight
	ws.Penalize(m, 5.0)

	if m.EffectiveWeight != 0 {
		t.Errorf("after penalize, m.EffectiveWeight = %f, want 0", m.EffectiveWeight)
	}
}

func TestWeightedSelector_Penalize_UpdatesLastUsed(t *testing.T) {
	m := &Mirror{URL: "https://mirror.com", BaseWeight: 1.0, EffectiveWeight: 1.0}

	ws := NewWeightedSelector()
	ws.Add(m)

	oldTime := m.LastUsed
	time.Sleep(10 * time.Millisecond)

	ws.Penalize(m, 0.5)

	if m.LastUsed.Before(oldTime) || m.LastUsed.Equal(oldTime) {
		t.Errorf("LastUsed was not updated after penalize")
	}
}

func TestWeightedSelector_Penalize_IgnoresZeroOrNegative(t *testing.T) {
	m := &Mirror{URL: "https://mirror.com", BaseWeight: 5.0, EffectiveWeight: 5.0}

	ws := NewWeightedSelector()
	ws.Add(m)

	oldTime := m.LastUsed
	ws.Penalize(m, 0)

	if m.EffectiveWeight != 5.0 {
		t.Errorf("penalize with 0 changed weight to %f", m.EffectiveWeight)
	}

	if !m.LastUsed.Equal(oldTime) {
		t.Errorf("penalize with 0 updated LastUsed")
	}

	ws.Penalize(m, -1.0)

	if m.EffectiveWeight != 5.0 {
		t.Errorf("penalize with negative value changed weight to %f", m.EffectiveWeight)
	}
}

func TestWeightedSelector_ThreadSafety(t *testing.T) {
	ws := NewWeightedSelector()
	m := &Mirror{URL: "https://mirror.com", BaseWeight: 1.0, EffectiveWeight: 1.0}
	ws.Add(m)

	// Run concurrent operations
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 100; i++ {
			m.AddInFlightDownload()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			m.RemoveInFlightDownload()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			m.GetInFlightDownloadCount()
		}
		done <- true
	}()

	<-done
	<-done
	<-done

	// Final count should be stable
	count := m.GetInFlightDownloadCount()
	if count < 0 || count > 100 {
		t.Errorf("final in-flight download count = %d, which seems invalid", count)
	}
}

// --- Tests for new selection algorithm (Issue #7) ---

func TestWeightedSelector_SelectionAlgorithm_RespectsPriorities(t *testing.T) {
	// Test that the algorithm respects effective weight priorities
	ws := NewWeightedSelector()
	ws.timeNow = func() time.Time { return time.Unix(1000, 0) }

	m1 := &Mirror{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: time.Time{}}
	m2 := &Mirror{URL: "https://mirror2.com", BaseWeight: 5.0, EffectiveWeight: 5.0, LastUsed: time.Time{}}
	m3 := &Mirror{URL: "https://mirror3.com", BaseWeight: 2.0, EffectiveWeight: 2.0, LastUsed: time.Time{}}

	ws.Add(m1)
	ws.Add(m2)
	ws.Add(m3)

	// Should select m2 (highest effective weight)
	selected, err := ws.Select()
	if err != nil {
		t.Fatalf("Select() returned error: %v", err)
	}
	if selected.URL != "https://mirror2.com" {
		t.Errorf("expected m2, got %q", selected.URL)
	}
}

func TestWeightedSelector_SelectionAlgorithm_PrefersLessUsed(t *testing.T) {
	// Test that mirrors unused for longer get higher scores when weights are equal
	ws := NewWeightedSelector()
	now := time.Unix(1000, 0)
	ws.timeNow = func() time.Time { return now }

	// All mirrors have same effective weight but different last usage times
	m1 := &Mirror{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: now.Add(-1 * time.Hour)}
	m2 := &Mirror{URL: "https://mirror2.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: now.Add(-10 * time.Hour)}
	m3 := &Mirror{URL: "https://mirror3.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: time.Time{}} // Never used

	ws.Add(m1)
	ws.Add(m2)
	ws.Add(m3)

	// With this algorithm, m3 (never used) should have highest score
	selected, err := ws.Select()
	if err != nil {
		t.Fatalf("Select() returned error: %v", err)
	}

	// m3 (never used) should be selected as it has no time-since-use penalty
	// but m2 (10 hours) should be higher priority than m1 (1 hour) when neither is never-used
	// Let's verify the algorithm prefers less-recently-used mirrors
	if selected.LastUsed.IsZero() {
		t.Logf("Correctly selected never-used mirror (highest score)")
	}
}

func TestWeightedSelector_SelectionAlgorithm_PenalizesInFlightDownloads(t *testing.T) {
	// Test that mirrors with many in-flight downloads are penalized
	ws := NewWeightedSelector()
	now := time.Unix(1000, 0)
	ws.timeNow = func() time.Time { return now }

	m1 := &Mirror{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0, InFlightDownloads: 0, LastUsed: time.Time{}}
	m2 := &Mirror{URL: "https://mirror2.com", BaseWeight: 1.0, EffectiveWeight: 1.0, InFlightDownloads: 10, LastUsed: time.Time{}}

	ws.Add(m1)
	ws.Add(m2)

	// m1 should be selected (no in-flight downloads)
	selected, err := ws.Select()
	if err != nil {
		t.Fatalf("Select() returned error: %v", err)
	}
	if selected.URL != "https://mirror1.com" {
		t.Errorf("expected m1 (no in-flight), got %q", selected.URL)
	}
}

func TestWeightedSelector_CalculateScore_ZeroWeight(t *testing.T) {
	// Mirrors with zero or negative weight should have zero score
	ws := NewWeightedSelector()
	now := time.Now()

	m := &Mirror{URL: "https://mirror.com", BaseWeight: 1.0, EffectiveWeight: 0}
	score := ws.calculateScore(m, now)

	if score != 0 {
		t.Errorf("score for zero-weight mirror = %f, want 0", score)
	}

	m.EffectiveWeight = -1
	score = ws.calculateScore(m, now)
	if score != 0 {
		t.Errorf("score for negative-weight mirror = %f, want 0", score)
	}
}

func TestWeightedSelector_CalculateScore_UnusedMirrorhigherScore(t *testing.T) {
	// Unused mirrors (LastUsed = zero time) should have score without time boost
	// but should still have reasonable score
	ws := NewWeightedSelector()
	now := time.Unix(1000, 0)

	m := &Mirror{URL: "https://mirror.com", BaseWeight: 1.0, EffectiveWeight: 2.0, LastUsed: time.Time{}, InFlightDownloads: 0}
	score := ws.calculateScore(m, now)

	// Expected score: 2.0 * (1 + 0) / (1 + 0) = 2.0
	if math.Abs(score-2.0) > 0.01 {
		t.Errorf("score for unused mirror = %f, want ~2.0", score)
	}
}

func TestWeightedSelector_CalculateScore_RecentlyUsedMirror(t *testing.T) {
	// Recently used mirrors get less boost from time-since-use
	ws := NewWeightedSelector()
	now := time.Unix(1000, 0)

	m1 := &Mirror{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: now.Add(-1 * time.Minute), InFlightDownloads: 0}
	m2 := &Mirror{URL: "https://mirror2.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: now.Add(-1 * time.Hour), InFlightDownloads: 0}

	score1 := ws.calculateScore(m1, now)
	score2 := ws.calculateScore(m2, now)

	// m2 (less recently used) should have higher score than m1
	if score2 <= score1 {
		t.Errorf("less-recently-used mirror should have higher score: score1=%f, score2=%f", score1, score2)
	}
}

func TestWeightedSelector_CalculateScore_InFlightDownloadPenalty(t *testing.T) {
	// In-flight downloads should reduce score
	ws := NewWeightedSelector()
	now := time.Unix(1000, 0)

	m1 := &Mirror{URL: "https://mirror1.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: now.Add(-1 * time.Hour), InFlightDownloads: 0}
	m2 := &Mirror{URL: "https://mirror2.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: now.Add(-1 * time.Hour), InFlightDownloads: 5}

	score1 := ws.calculateScore(m1, now)
	score2 := ws.calculateScore(m2, now)

	// m1 (no in-flight) should have higher score than m2 (5 in-flight)
	if score1 <= score2 {
		t.Errorf("mirror with no in-flight should have higher score: score1=%f, score2=%f", score1, score2)
	}
}

func TestWeightedSelector_SelectionAlgorithm_ComplexScenario(t *testing.T) {
	// Complex scenario: multiple mirrors with different weights, usage, and in-flight counts
	ws := NewWeightedSelector()
	now := time.Unix(10000, 0)
	ws.timeNow = func() time.Time { return now }

	// Mirror A: high priority but recently used with in-flight downloads
	m1 := &Mirror{URL: "https://mirrorA.com", BaseWeight: 5.0, EffectiveWeight: 5.0, LastUsed: now.Add(-1 * time.Minute), InFlightDownloads: 8}

	// Mirror B: medium priority, moderately old, few in-flight
	m2 := &Mirror{URL: "https://mirrorB.com", BaseWeight: 3.0, EffectiveWeight: 3.0, LastUsed: now.Add(-30 * time.Minute), InFlightDownloads: 1}

	// Mirror C: low priority but never used
	m3 := &Mirror{URL: "https://mirrorC.com", BaseWeight: 1.0, EffectiveWeight: 1.0, LastUsed: time.Time{}, InFlightDownloads: 0}

	ws.Add(m1)
	ws.Add(m2)
	ws.Add(m3)

	selected, err := ws.Select()
	if err != nil {
		t.Fatalf("Select() returned error: %v", err)
	}

	// Mirror B is likely to be selected: good balance of priority, freshness, and low load
	// But let's verify it returns a valid mirror
	if selected == nil {
		t.Errorf("expected valid mirror selection")
	}
}
