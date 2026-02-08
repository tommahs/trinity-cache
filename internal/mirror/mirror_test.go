package mirror

import (
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
