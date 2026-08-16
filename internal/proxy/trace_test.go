package proxy

import (
	"testing"
	"time"
)

func TestPhaseTrace_TimingFromHooks(t *testing.T) {
	base := time.Now()
	tr := &phaseTrace{
		dnsStart:      base,
		dnsDone:       base.Add(2 * time.Millisecond),
		connectStart:  base.Add(2 * time.Millisecond),
		connectDone:   base.Add(5 * time.Millisecond),
		wroteRequest:  base.Add(6 * time.Millisecond),
		firstByte:     base.Add(40 * time.Millisecond),
	}
	got := tr.timing(base, base.Add(50*time.Millisecond))
	if got.DNS != 2*time.Millisecond {
		t.Errorf("DNS = %s", got.DNS)
	}
	if got.Connect != 3*time.Millisecond {
		t.Errorf("Connect = %s", got.Connect)
	}
	if got.Wait != 34*time.Millisecond {
		t.Errorf("Wait = %s", got.Wait)
	}
	if got.Transfer != 10*time.Millisecond {
		t.Errorf("Transfer = %s", got.Transfer)
	}
	if got.TLS != 0 {
		t.Errorf("TLS = %s, want 0 on plain HTTP", got.TLS)
	}
}
