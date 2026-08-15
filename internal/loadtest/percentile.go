package loadtest

import (
	"sort"
	"time"
)

// Percentiles summarizes a set of latency samples. Computed via the
// "nearest-rank" method (sort, then index at ceil(p/100*n)-1) — simple,
// deterministic, and avoids pulling in a stats dependency for what is
// fundamentally "sort N numbers and pick some indexes."
type Percentiles struct {
	Min  time.Duration
	Mean time.Duration
	P50  time.Duration
	P90  time.Duration
	P95  time.Duration
	P99  time.Duration
	Max  time.Duration
}

// computePercentiles sorts a COPY of samples (never mutates the caller's
// slice) and returns the standard percentile cuts. Returns the zero value
// for an empty input rather than panicking or dividing by zero.
func computePercentiles(samples []time.Duration) Percentiles {
	if len(samples) == 0 {
		return Percentiles{}
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	return Percentiles{
		Min:  sorted[0],
		Mean: sum / time.Duration(len(sorted)),
		P50:  percentileAt(sorted, 50),
		P90:  percentileAt(sorted, 90),
		P95:  percentileAt(sorted, 95),
		P99:  percentileAt(sorted, 99),
		Max:  sorted[len(sorted)-1],
	}
}

// percentileAt returns the value at percentile p (0-100) from an
// already-sorted slice using nearest-rank: index = ceil(p/100*n) - 1,
// clamped to [0, n-1].
func percentileAt(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	rank := int((p/100)*float64(n) + 0.9999999) // ceil without importing math for one call
	idx := rank - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
