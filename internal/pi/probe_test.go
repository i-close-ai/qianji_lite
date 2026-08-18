package pi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/pi"
)

func TestFilterCatalogProbeAllKeepsOnlySuccesses(t *testing.T) {
	catalog := []config.CatalogItem{
		{Provider: "aibase", Model: "kimi-k3"},
		{Provider: "anthropic", Model: "claude-opus-4-8"},
		{Provider: "onehz", Model: "gpt-5.6-terra"},
	}
	report, err := pi.FilterCatalog(catalog, pi.FilterOpts{
		ProbeAll: true,
		Probe: func(item config.CatalogItem) pi.ExecResult {
			if item.Provider == "anthropic" {
				return pi.ExecResult{OK: false, Error: "Error: HTTP 401", ErrorType: "provider_failure"}
			}
			return pi.ExecResult{OK: true, Output: "pong"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Probed != 3 {
		t.Fatalf("probed=%d", report.Probed)
	}
	if len(report.Live) != 2 || report.Live[0].Model != "kimi-k3" || report.Live[1].Model != "gpt-5.6-terra" {
		t.Fatalf("live=%+v", report.Live)
	}
	if len(report.Failed) != 1 || report.Failed[0].Ref != "anthropic/claude-opus-4-8" {
		t.Fatalf("failed=%+v", report.Failed)
	}
	failed := pi.FailedRefs(report, catalog, nil, true)
	if len(failed) != 1 || failed[0] != "anthropic/claude-opus-4-8" {
		t.Fatalf("failed refs=%v", failed)
	}
}

func TestFilterCatalogProbeNewSkipsCachedFailuresAndExisting(t *testing.T) {
	catalog := []config.CatalogItem{
		{Provider: "aibase", Model: "kimi-k3"},
		{Provider: "anthropic", Model: "claude-opus-4-8"},
		{Provider: "aibase", Model: "glm-5-2"},
	}
	probed := []string{}
	report, err := pi.FilterCatalog(catalog, pi.FilterOpts{
		ProbeAll: false,
		Existing: map[string]struct{}{
			config.RouteKey("aibase", "kimi-k3"): {},
		},
		CachedFailed: map[string]struct{}{
			config.RouteKey("anthropic", "claude-opus-4-8"): {},
		},
		Probe: func(item config.CatalogItem) pi.ExecResult {
			probed = append(probed, pi.ItemRef(item.Provider, item.Model))
			return pi.ExecResult{OK: true, Output: "pong"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(probed) != 1 || probed[0] != "aibase/glm-5-2" {
		t.Fatalf("probed=%v", probed)
	}
	if report.Probed != 1 {
		t.Fatalf("report.probed=%d", report.Probed)
	}
	if len(report.Live) != 2 {
		t.Fatalf("live=%+v", report.Live)
	}
	if len(report.SkippedFail) != 1 || report.SkippedFail[0] != "anthropic/claude-opus-4-8" {
		t.Fatalf("skipped=%v", report.SkippedFail)
	}
	failed := pi.FailedRefs(report, catalog, map[string]struct{}{
		config.RouteKey("anthropic", "claude-opus-4-8"): {},
	}, false)
	if len(failed) != 1 || failed[0] != "anthropic/claude-opus-4-8" {
		t.Fatalf("failed refs=%v", failed)
	}
}

func TestProbeRouteUsesCheapFlags(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	stub := filepath.Join(dir, "pi")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$QIANJI_PROBE_ARGS\"\necho pong\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("QIANJI_PROBE_ARGS", argsFile)

	cfg := config.Default()
	res := pi.ProbeRoute(cfg, config.CatalogItem{Provider: "aibase", Model: "kimi-k3"}, dir, 5)
	if !res.OK {
		t.Fatalf("probe failed: %+v", res)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{
		"--provider", "aibase",
		"--model", "kimi-k3",
		"--print",
		"--no-session",
		"--no-tools",
		"--thinking", "off",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q in args:\n%s", want, args)
		}
	}
}
