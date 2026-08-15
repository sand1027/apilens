package loadtest

import (
	"testing"
	"time"
)

func TestComputePercentiles_EmptyInputReturnsZeroValue(t *testing.T) {
	p := computePercentiles(nil)
	if p != (Percentiles{}) {
		t.Errorf("expected zero value for empty input, got %+v", p)
	}
}

func TestComputePercentiles_SingleValue(t *testing.T) {
	p := computePercentiles([]time.Duration{10 * time.Millisecond})
	want := 10 * time.Millisecond
	if p.Min != want || p.Max != want || p.Mean != want || p.P50 != want || p.P99 != want {
		t.Errorf("expected every stat to equal the single sample, got %+v", p)
	}
}

func TestComputePercentiles_SortedRangeMatchesKnownRanks(t *testing.T) {
	// 1..100 ms — nearest-rank percentiles have exact, easily verified
	// answers for this input: P50 -> 50th smallest -> 50ms, P90 -> 90ms,
	// P95 -> 95ms, P99 -> 99ms.
	var samples []time.Duration
	for i := 1; i <= 100; i++ {
		samples = append(samples, time.Duration(i)*time.Millisecond)
	}
	p := computePercentiles(samples)
	if p.Min != 1*time.Millisecond {
		t.Errorf("Min = %v, want 1ms", p.Min)
	}
	if p.Max != 100*time.Millisecond {
		t.Errorf("Max = %v, want 100ms", p.Max)
	}
	if p.P50 != 50*time.Millisecond {
		t.Errorf("P50 = %v, want 50ms", p.P50)
	}
	if p.P90 != 90*time.Millisecond {
		t.Errorf("P90 = %v, want 90ms", p.P90)
	}
	if p.P95 != 95*time.Millisecond {
		t.Errorf("P95 = %v, want 95ms", p.P95)
	}
	if p.P99 != 99*time.Millisecond {
		t.Errorf("P99 = %v, want 99ms", p.P99)
	}
}

func TestComputePercentiles_DoesNotMutateInputSlice(t *testing.T) {
	samples := []time.Duration{5 * time.Millisecond, 1 * time.Millisecond, 3 * time.Millisecond}
	original := append([]time.Duration(nil), samples...)
	_ = computePercentiles(samples)
	for i := range samples {
		if samples[i] != original[i] {
			t.Errorf("input slice was mutated: got %v, want %v", samples, original)
		}
	}
}

func TestComputePercentiles_MeanIsArithmeticAverage(t *testing.T) {
	samples := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	p := computePercentiles(samples)
	want := 20 * time.Millisecond
	if p.Mean != want {
		t.Errorf("Mean = %v, want %v", p.Mean, want)
	}
}
