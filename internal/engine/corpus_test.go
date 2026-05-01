package engine

import (
	"testing"
	"time"
)

func TestDeriveParams_SmallRepo(t *testing.T) {
	stats := CorpusStats{
		CommitCount:    13,
		EventCount:     20,
		TimeSpanDays:   5,
		SignalDensity:  1.5,
		SourceMix:      map[string]int{},
		PlanCount:      0,
		MedianGapHours: 2.0,
	}

	p := DeriveParams(stats)

	if p.TopK != 15 {
		t.Errorf("TopK = %d, want 15 (clamped floor)", p.TopK)
	}
	if p.RecencyHalfLife != 7*24*time.Hour {
		t.Errorf("RecencyHalfLife = %v, want 7d (floor)", p.RecencyHalfLife)
	}
	if p.RenderCap < 5 || p.RenderCap > 20 {
		t.Errorf("RenderCap = %d, want [5,20]", p.RenderCap)
	}
}

func TestDeriveParams_LargeRepo(t *testing.T) {
	stats := CorpusStats{
		CommitCount:    5000,
		EventCount:     2000,
		TimeSpanDays:   365,
		SignalDensity:  0.4,
		SourceMix:      map[string]int{"cursor_plan": 50},
		PlanCount:      50,
		MedianGapHours: 8.0,
		IsSignalRich:   false,
	}

	p := DeriveParams(stats)

	if p.TopK != 200 {
		t.Errorf("TopK = %d, want 200 (clamped ceiling)", p.TopK)
	}
	wantHalfLife := time.Duration(36.5*24) * time.Hour
	if p.RecencyHalfLife != wantHalfLife {
		t.Errorf("RecencyHalfLife = %v, want %v", p.RecencyHalfLife, wantHalfLife)
	}
	if p.RenderCap != 20 {
		t.Errorf("RenderCap = %d, want 20 (clamped ceiling)", p.RenderCap)
	}
}

func TestDeriveParams_MediumRepo(t *testing.T) {
	stats := CorpusStats{
		CommitCount:    200,
		EventCount:     300,
		TimeSpanDays:   90,
		SignalDensity:  1.5,
		SourceMix:      map[string]int{"cursor_plan": 10},
		PlanCount:      10,
		MedianGapHours: 4.0,
		IsSignalRich:   true,
	}

	p := DeriveParams(stats)

	wantTopK := 45 // 300 * 0.15 = 45
	if p.TopK != wantTopK {
		t.Errorf("TopK = %d, want %d", p.TopK, wantTopK)
	}
	if p.RenderCap != 20 {
		t.Errorf("RenderCap = %d, want 20", p.RenderCap)
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		min, val, max, want int
	}{
		{5, 3, 20, 5},
		{5, 10, 20, 10},
		{5, 25, 20, 20},
	}
	for _, tc := range tests {
		got := clampInt(tc.min, tc.val, tc.max)
		if got != tc.want {
			t.Errorf("clampInt(%d,%d,%d) = %d, want %d", tc.min, tc.val, tc.max, got, tc.want)
		}
	}
}
