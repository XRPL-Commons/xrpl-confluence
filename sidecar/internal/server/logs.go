package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// LogLine is one NDJSON record emitted by GET /v1/logs.
type LogLine struct {
	Ts      string `json:"ts"`
	Level   string `json:"level,omitempty"`
	Node    string `json:"node"`
	Message string `json:"message"`
}

// nodeRe is the allowlist for the node query parameter.
var nodeRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// logTsRe matches a leading RFC3339-millis timestamp: 2026-05-12T14:00:00.000Z
var logTsRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s+`)

// logLevelRe matches a bracketed level token: [info], [warn], etc.
var logLevelRe = regexp.MustCompile(`^\[([a-zA-Z]+)\]\s*`)

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	node := q.Get("node")
	if !nodeRe.MatchString(node) {
		writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeBadRequest, Message: "node must match ^[a-z0-9-]+$", Field: "node"},
		})
		return
	}

	var sinceFilter time.Duration
	if raw := q.Get("since"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
				Error: api.Error{Code: api.ErrCodeBadRequest, Message: "since must be a Go duration (e.g. 30s, 5m)", Field: "since"},
			})
			return
		}
		sinceFilter = d
	}

	var grepRe *regexp.Regexp
	if raw := q.Get("grep"); raw != "" {
		re, err := regexp.Compile(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
				Error: api.Error{Code: api.ErrCodeBadRequest, Message: "grep is not a valid regex", Field: "grep"},
			})
			return
		}
		grepRe = re
	}

	follow := q.Get("follow") == "true" || q.Get("follow") == "1"

	limit := 1000
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
				Error: api.Error{Code: api.ErrCodeBadRequest, Message: "limit must be a positive integer", Field: "limit"},
			})
			return
		}
		if n > 10000 {
			n = 10000
		}
		limit = n
	}

	logPath := filepath.Join(s.logsDir, node+".log")
	f, err := os.Open(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, api.ErrorResponse{
				Error: api.Error{Code: api.ErrCodeLogsNotFound, Message: "log file not found", Field: "node"},
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeBadRequest, Message: "could not open log file"},
		})
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/x-ndjson")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)
	enc := json.NewEncoder(w)

	cutoff := time.Time{}
	if sinceFilter > 0 {
		cutoff = time.Now().Add(-sinceFilter)
	}

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		line := scanner.Text()
		ll := tryParseLine(line, node)

		if !cutoff.IsZero() {
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", ll.Ts); err == nil {
				if t.Before(cutoff) {
					continue
				}
			}
		}

		if grepRe != nil && !grepRe.MatchString(ll.Message) {
			continue
		}

		enc.Encode(ll)
		if canFlush {
			flusher.Flush()
		}
		count++
		if !follow && count >= limit {
			return
		}
	}

	if !follow {
		return
	}

	// Follow mode: poll for new content every 500ms.
	offset, _ := f.Seek(0, io.SeekCurrent)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			info, err := f.Stat()
			if err != nil {
				return
			}
			if info.Size() <= offset {
				continue
			}

			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				return
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				select {
				case <-r.Context().Done():
					return
				default:
				}

				line := scanner.Text()
				ll := tryParseLine(line, node)

				if grepRe != nil && !grepRe.MatchString(ll.Message) {
					offset += int64(len(line)) + 1
					continue
				}

				enc.Encode(ll)
				if canFlush {
					flusher.Flush()
				}
				offset += int64(len(line)) + 1
			}
		}
	}
}

// tryParseLine parses a raw log line into a LogLine.
// Format attempted: `YYYY-MM-DDTHH:MM:SS.fffZ [LEVEL] message`
// Unparseable lines get Ts=now, Level="", Message=raw.
func tryParseLine(raw, node string) LogLine {
	rest := raw
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	level := ""

	if m := logTsRe.FindStringSubmatchIndex(rest); m != nil {
		ts = raw[m[2]:m[3]]
		rest = raw[m[1]:]
	} else {
		return LogLine{Ts: ts, Node: node, Message: raw}
	}

	if m := logLevelRe.FindStringSubmatchIndex(rest); m != nil {
		level = strings.ToLower(rest[m[2]:m[3]])
		rest = rest[m[1]:]
	}

	return LogLine{Ts: ts, Level: level, Node: node, Message: rest}
}
