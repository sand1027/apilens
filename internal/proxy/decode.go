package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
)

// decodeCapturedBody unpacks Content-Encoding for the stored copy only.
// The bytes forwarded to the browser stay compressed.
func decodeCapturedBody(encoding string, body []byte) []byte {
	if len(body) == 0 || strings.TrimSpace(encoding) == "" {
		return body
	}
	out := body
	parts := strings.Split(encoding, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		enc := strings.ToLower(strings.TrimSpace(parts[i]))
		next, err := decodeOne(enc, out)
		if err != nil {
			return body
		}
		out = next
	}
	return out
}

func decodeOne(enc string, body []byte) ([]byte, error) {
	switch enc {
	case "", "identity":
		return body, nil
	case "gzip", "x-gzip":
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	case "deflate":
		r := flate.NewReader(bytes.NewReader(body))
		defer r.Close()
		return io.ReadAll(r)
	case "br":
		return io.ReadAll(brotli.NewReader(bytes.NewReader(body)))
	default:
		return body, nil
	}
}
