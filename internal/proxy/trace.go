package proxy

import (
	"crypto/tls"
	"net/http/httptrace"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

// phaseTrace records httptrace timestamps on the proxy→upstream hop
// (the same phases hoptrace shows: DNS, connect, TLS, wait, transfer).
type phaseTrace struct {
	dnsStart, dnsDone         time.Time
	connectStart, connectDone time.Time
	tlsStart, tlsDone         time.Time
	gotConn, wroteRequest     time.Time
	firstByte                 time.Time
}

func (t *phaseTrace) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { t.dnsStart = time.Now() },
		DNSDone:              func(httptrace.DNSDoneInfo) { t.dnsDone = time.Now() },
		ConnectStart:         func(_, _ string) { t.connectStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { t.connectDone = time.Now() },
		TLSHandshakeStart:    func() { t.tlsStart = time.Now() },
		TLSHandshakeDone:     func(_ tls.ConnectionState, _ error) { t.tlsDone = time.Now() },
		GotConn:              func(httptrace.GotConnInfo) { t.gotConn = time.Now() },
		WroteRequest:         func(httptrace.WroteRequestInfo) { t.wroteRequest = time.Now() },
		GotFirstResponseByte: func() { t.firstByte = time.Now() },
	}
}

func dur(a, b time.Time) time.Duration {
	if a.IsZero() || b.IsZero() || b.Before(a) {
		return 0
	}
	return b.Sub(a)
}

func (t *phaseTrace) timing(start, end time.Time) domain.Timing {
	waitStart := t.wroteRequest
	if waitStart.IsZero() {
		waitStart = t.gotConn
	}
	ttfbStart := t.dnsStart
	if ttfbStart.IsZero() {
		ttfbStart = t.connectStart
	}
	if ttfbStart.IsZero() {
		ttfbStart = start
	}
	return domain.Timing{
		Start:    start,
		End:      end,
		Duration: end.Sub(start),
		DNS:      dur(t.dnsStart, t.dnsDone),
		Connect:  dur(t.connectStart, t.connectDone),
		TLS:      dur(t.tlsStart, t.tlsDone),
		Wait:     dur(waitStart, t.firstByte),
		TTFB:     dur(ttfbStart, t.firstByte),
		Transfer: dur(t.firstByte, end),
	}
}
