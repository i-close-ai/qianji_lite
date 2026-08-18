package runlog_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/runlog"
)

func TestSanitizeRedactsKeysAndTruncates(t *testing.T) {
	got := runlog.Sanitize("invalid api key sk-ant-abcdefghijklmnop", 400)
	if strings.Contains(got, "sk-ant-abcdefghijklmnop") {
		t.Fatalf("got %q", got)
	}
	long := runlog.Sanitize(strings.Repeat("x", 500), 400)
	if len(long) > 410 {
		t.Fatalf("len=%d", len(long))
	}
}

func TestSuccessLineIsCompact(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	if err := runlog.Append(runlog.Event{
		Pool:        "ordinary",
		RouteID:     "acme-fast",
		Circuit:     "acme:fast",
		Provider:    "acme",
		Model:       "fast",
		Effort:      "high",
		Via:         "weighted_random",
		Attempt:     1,
		MaxAttempts: 8,
		TimeoutSec:  900,
		ElapsedMs:   12,
		OK:          true,
		PromptBytes: 42,
		Affinity:    runlog.ShortAffinity("abc123def4567890"),
	}); err != nil {
		t.Fatal(err)
	}
	line := firstLine(t, config.RunLogPath())
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != true || m["route_id"] != "acme-fast" {
		t.Fatalf("%s", line)
	}
	for _, key := range []string{"timeout_sec", "prompt_bytes", "affinity", "circuit", "effort", "max_attempts", "error", "output_head"} {
		if _, ok := m[key]; ok {
			t.Fatalf("success log should omit %s: %s", key, line)
		}
	}
	if _, ok := m["attempt"]; ok {
		t.Fatalf("attempt 1 should be omitted: %s", line)
	}
}

func TestFailureLineIsDetailed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	if err := runlog.Append(runlog.Event{
		Pool:        "strong",
		RouteID:     "tier-strong",
		Provider:    "acme",
		Model:       "pro",
		Attempt:     1,
		TimeoutSec:  1800,
		ElapsedMs:   900000,
		OK:          false,
		ErrorType:   "timeout",
		Error:       "timeout after 1800s",
		OutputHead:  runlog.OutputHead("still working\n" + strings.Repeat("x", 80)),
		PromptBytes: 2048,
		Affinity:    "abc123def456",
	}); err != nil {
		t.Fatal(err)
	}
	line := firstLine(t, config.RunLogPath())
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != false || m["error_type"] != "timeout" {
		t.Fatalf("%s", line)
	}
	for _, key := range []string{"error", "output_head", "timeout_sec", "prompt_bytes", "elapsed_ms"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("failure log missing %s: %s", key, line)
		}
	}
}

func TestRotateCapsFileSize(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	restore := runlog.SetLimitsForTest(120, 2)
	t.Cleanup(restore)
	for i := 0; i < 20; i++ {
		if err := runlog.Append(runlog.Event{OK: true, RouteID: "acme-fast", ElapsedMs: 1}); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(config.RunLogPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 200 {
		t.Fatalf("active log too large: %d", info.Size())
	}
	if _, err := os.Stat(config.RunLogPath() + ".1"); err != nil {
		t.Fatalf("expected rotated .1: %v", err)
	}
}

func firstLine(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected one line")
	}
	return sc.Text()
}

func TestRunLogPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	if got := config.RunLogPath(); got != filepath.Join(dir, "logs", "runs.jsonl") {
		t.Fatalf("path=%s", got)
	}
}
