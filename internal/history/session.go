package history

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

// SessionFile implements the cross-terminal replay mechanism from
// docs/08-proxy.md section 3: watch writes already-redacted JSONL as it
// captures; a second terminal's `replay`/`generate`/`history` read the
// file when in-memory history is empty (a different process).
type SessionFile struct {
	path string
}

// sessionRecord is the on-disk JSONL shape. Kept separate from
// domain.Exchange so header maps serialize predictably and so the file
// format doesn't silently change if domain.Exchange's Go shape changes.
type sessionRecord struct {
	Display         int                 `json:"display_id"`
	ID              string              `json:"id"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	RequestHeaders  map[string][]string `json:"request_headers,omitempty"`
	RequestBody     string              `json:"request_body,omitempty"`
	StatusCode      int                 `json:"status_code"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
	ResponseBody    string              `json:"response_body,omitempty"`
	DurationMS      int64               `json:"duration_ms"`
	Timestamp       time.Time           `json:"timestamp"`
	Truncated       bool                `json:"truncated"`
	Redacted        bool                `json:"redacted"`
	Error           string              `json:"error,omitempty"`
}

// DefaultPath implements docs/08-proxy.md section 3's documented location:
// $APILENS_HISTORY_FILE, or /tmp/apilens-history-<project-hash>.jsonl.
func DefaultPath(projectDir string) string {
	if v := os.Getenv("APILENS_HISTORY_FILE"); v != "" {
		return v
	}
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		abs = projectDir
	}
	sum := sha1.Sum([]byte(abs))
	hash := hex.EncodeToString(sum[:])[:8]
	return filepath.Join(os.TempDir(), fmt.Sprintf("apilens-history-%s.jsonl", hash))
}

// NewSessionFile opens (creating if needed) a session file at path with
// mode 0600, per docs/09-security.md section 6 ("mode 0600 ... contents
// already redacted ... do not put it in the repo").
func NewSessionFile(path string) (*SessionFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE, 0o600)
	if err != nil {
		return nil, domain.NewConfigError("creating session history file "+path, err)
	}
	_ = f.Close()
	return &SessionFile{path: path}, nil
}

// Path returns the session file's location, printed in the watch banner.
func (s *SessionFile) Path() string { return s.path }

// Append writes one exchange as a JSONL line. The caller is responsible
// for having already redacted ex (SessionFile does not redact — that
// policy lives in internal/security and is applied once, before both the
// in-memory store and the session file, per docs/09-security.md section 3).
func (s *SessionFile) Append(ex domain.Exchange) error {
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return domain.NewConfigError("opening session history file", err)
	}
	defer f.Close()

	rec := toRecord(ex)
	line, err := json.Marshal(rec)
	if err != nil {
		return domain.NewConfigError("marshaling history record", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return domain.NewConfigError("writing history record", err)
	}
	return nil
}

// ReadAll parses every JSONL line in the session file back into
// exchanges, in append order. A missing file returns (nil, nil) — no
// session yet is not an error.
func ReadAll(path string) ([]domain.Exchange, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, domain.NewConfigError("reading session history file", err)
	}
	defer f.Close()

	var out []domain.Exchange
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024) // allow larger truncated bodies
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec sessionRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue // skip a corrupt line rather than fail the whole read
		}
		out = append(out, fromRecord(rec))
	}
	if err := scanner.Err(); err != nil {
		return nil, domain.NewConfigError("scanning session history file", err)
	}
	return out, nil
}

func toRecord(ex domain.Exchange) sessionRecord {
	rec := sessionRecord{
		Display:      int(ex.Display),
		ID:           string(ex.ID),
		Method:       string(ex.Request.Method),
		URL:          ex.Request.URL,
		RequestBody:  string(ex.Request.Body),
		StatusCode:   ex.Response.StatusCode,
		ResponseBody: string(ex.Response.Body),
		DurationMS:   ex.Timing.Duration.Milliseconds(),
		Timestamp:    ex.Timestamp,
		Truncated:    ex.Response.Truncated,
		Redacted:     ex.Redacted,
	}
	if ex.Request.Headers != nil {
		rec.RequestHeaders = map[string][]string(ex.Request.Headers)
	}
	if ex.Response.Headers != nil {
		rec.ResponseHeaders = map[string][]string(ex.Response.Headers)
	}
	if ex.Err != nil {
		rec.Error = ex.Err.Error()
	}
	return rec
}

func fromRecord(rec sessionRecord) domain.Exchange {
	ex := domain.Exchange{
		ID:      domain.ExchangeID(rec.ID),
		Display: domain.DisplayID(rec.Display),
		Request: domain.HTTPRequest{
			Method:  domain.Method(rec.Method),
			URL:     rec.URL,
			Headers: http.Header(rec.RequestHeaders),
			Body:    []byte(rec.RequestBody),
		},
		Response: domain.HTTPResponse{
			StatusCode: rec.StatusCode,
			Headers:    http.Header(rec.ResponseHeaders),
			Body:       []byte(rec.ResponseBody),
			Truncated:  rec.Truncated,
		},
		Timing:    domain.Timing{Duration: time.Duration(rec.DurationMS) * time.Millisecond},
		Timestamp: rec.Timestamp,
		Redacted:  rec.Redacted,
	}
	if rec.Error != "" {
		ex.Err = fmt.Errorf("%s", rec.Error)
	}
	return ex
}
