package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
)

const (
	defaultMaxFileBytes int64 = 4 << 20
	defaultKeepRotated        = 4
)

var (
	maxFileBytes = defaultMaxFileBytes
	keepRotated  = defaultKeepRotated
)

// Event is one Pi attempt. Prompt text and API keys must never be stored.
// Success records are compacted; failures keep extra diagnostic fields.
type Event struct {
	TS          string `json:"ts"`
	Pool        string `json:"pool,omitempty"`
	RouteID     string `json:"route_id,omitempty"`
	Circuit     string `json:"circuit,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	Via         string `json:"via,omitempty"`
	Attempt     int    `json:"attempt,omitempty"`
	MaxAttempts int    `json:"max_attempts,omitempty"`
	TimeoutSec  int    `json:"timeout_sec,omitempty"`
	ElapsedMs   int64  `json:"elapsed_ms,omitempty"`
	OK          bool   `json:"ok"`
	ErrorType   string `json:"error_type,omitempty"`
	Error       string `json:"error,omitempty"`
	OutputHead  string `json:"output_head,omitempty"`
	PromptBytes int    `json:"prompt_bytes,omitempty"`
	Affinity    string `json:"affinity,omitempty"`
}

type compactEvent struct {
	TS        string `json:"ts"`
	OK        bool   `json:"ok"`
	Pool      string `json:"pool,omitempty"`
	RouteID   string `json:"route_id,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Via       string `json:"via,omitempty"`
	Attempt   int    `json:"attempt,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms,omitempty"`
}

var keyLike = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]{10,}|api[_-]?key\s*[:=]\s*\S+)`)

func Sanitize(s string, max int) string {
	s = strings.TrimSpace(s)
	s = keyLike.ReplaceAllString(s, "[redacted]")
	if max <= 0 {
		max = 400
	}
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func OutputHead(output string) string {
	text := strings.TrimSpace(output)
	if text == "" {
		return ""
	}
	var b strings.Builder
	n := 0
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if n > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		n++
		if n >= 12 || b.Len() >= 1500 {
			break
		}
	}
	return b.String()
}

func ShortAffinity(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func encode(ev Event) ([]byte, error) {
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339)
	}
	var payload any
	if ev.OK && ev.ErrorType == "" {
		c := compactEvent{
			TS:        ev.TS,
			OK:        true,
			Pool:      ev.Pool,
			RouteID:   ev.RouteID,
			Provider:  ev.Provider,
			Model:     ev.Model,
			Via:       ev.Via,
			ElapsedMs: ev.ElapsedMs,
		}
		if ev.Attempt > 1 {
			c.Attempt = ev.Attempt
		}
		payload = c
	} else {
		ev.Error = Sanitize(ev.Error, 800)
		ev.OutputHead = Sanitize(ev.OutputHead, 1500)
		payload = ev
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func Append(ev Event) error {
	line, err := encode(ev)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.LogDir(), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(config.LogDir(), ".runs.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	path := config.RunLogPath()
	if err := rotateIfNeeded(path, int64(len(line))); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

func rotateIfNeeded(path string, nextBytes int64) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() == 0 || info.Size()+nextBytes < maxFileBytes {
		return nil
	}
	oldest := fmt.Sprintf("%s.%d", path, keepRotated)
	_ = os.Remove(oldest)
	for i := keepRotated - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", path, i)
		to := fmt.Sprintf("%s.%d", path, i+1)
		_ = os.Rename(from, to)
	}
	return os.Rename(path, path+".1")
}

// SetLimitsForTest overrides rotation limits. The returned func restores them.
func SetLimitsForTest(maxBytes int64, keep int) func() {
	oldMax, oldKeep := maxFileBytes, keepRotated
	maxFileBytes = maxBytes
	keepRotated = keep
	return func() {
		maxFileBytes = oldMax
		keepRotated = oldKeep
	}
}
