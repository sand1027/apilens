package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// maxHistoryLine is an upper bound on one JSONL record. Bodies are already
// capped by max_response_size (default 5MB); this is a safety net so a
// huge leftover Chrome download cannot fail the whole session read.
const maxHistoryLine = 12 * 1024 * 1024

// DefaultPath implements docs/08-proxy.md section 3's documented location:
// $APILENS_HISTORY_FILE, or <project>/.apilens/history/session.jsonl.
// The file is gitignored; watch and history in a second terminal must
// resolve the same project directory or they will miss each other.
func DefaultPath(projectDir string) string {
	if v := os.Getenv("APILENS_HISTORY_FILE"); v != "" {
		return v
	}
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		abs = projectDir
	}
	return filepath.Join(abs, ".apilens", "history", "session.jsonl")
}

func pointerFile() string {
	if v := os.Getenv("APILENS_HISTORY_POINTER"); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(), "apilens-current-history")
}

// SetActivePath records the session file the running `watch` process is
// writing. `apilens ui` started from a different repo (Stance frontend vs
// API) follows this pointer so the overlay and dashboard see the same hits.
func SetActivePath(sessionPath string) error {
	if sessionPath == "" {
		return nil
	}
	abs, err := filepath.Abs(sessionPath)
	if err != nil {
		abs = sessionPath
	}
	path := pointerFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(abs+"\n"), 0o600)
}

// ActivePath is the session file last advertised by watch, or "".
func ActivePath() string {
	b, err := os.ReadFile(pointerFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// SearchPaths is the order HistoryList tries: this project's session
// file, then the active watch session if it is a different path.
func SearchPaths(projectDir string) []string {
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		for _, e := range out {
			if e == p {
				return
			}
		}
		out = append(out, p)
	}
	add(DefaultPath(projectDir))
	add(ActivePath())
	return out
}

// NewSessionFile creates the session file at path with mode 0600, per
// docs/09-security.md section 6 ("mode 0600 ... contents already redacted
// ... do not put it in the repo"). A new watch session truncates any
// leftover file so display IDs start at #1 without colliding with a
// previous run's records.
func NewSessionFile(path string) (*SessionFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, domain.NewConfigError("creating session history directory", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
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
// session yet is not an error. Corrupt or oversized lines are skipped
// so one Chrome download cannot hide a later GraphQL capture.
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
	br := bufio.NewReader(f)
	for {
		line, err := readJSONLLine(br, maxHistoryLine)
		if len(line) > 0 {
			var rec sessionRecord
			if json.Unmarshal(line, &rec) == nil {
				out = append(out, fromRecord(rec))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, domain.NewConfigError("scanning session history file", err)
		}
	}
	return out, nil
}

// readJSONLLine returns one line without the trailing newline. Lines
// larger than max are discarded (empty slice) rather than unmarshaled.
func readJSONLLine(r *bufio.Reader, max int) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if len(line) == 0 && err == io.EOF {
		return nil, io.EOF
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > max {
		return nil, err
	}
	return line, err
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
