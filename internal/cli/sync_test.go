package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/pi"
	"github.com/i-close-ai/qianji_lite/internal/state"
)

func TestSyncFromPiInitKeepsOnlyProbeSuccesses(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	t.Setenv("QIANJI_CONFIG", filepath.Join(dir, "config.toml"))

	orig := catalogFilter
	t.Cleanup(func() { catalogFilter = orig })
	catalogFilter = func(catalog []config.CatalogItem, opts pi.FilterOpts) (pi.ProbeReport, error) {
		return pi.FilterCatalog(catalog, pi.FilterOpts{
			ProbeAll:     opts.ProbeAll,
			Existing:     opts.Existing,
			CachedFailed: opts.CachedFailed,
			Probe: func(item config.CatalogItem) pi.ExecResult {
				if item.Provider == "anthropic" {
					return pi.ExecResult{OK: false, Error: "Error: HTTP 401", ErrorType: "provider_failure"}
				}
				return pi.ExecResult{OK: true, Output: "pong"}
			},
		})
	}

	catalog := []config.CatalogItem{
		{Provider: "aibase", Model: "kimi-k3"},
		{Provider: "anthropic", Model: "claude-opus-4-8"},
		{Provider: "onehz", Model: "gpt-5.6-terra"},
	}
	result, err := syncFromPi(syncOpts{
		reason:      "init",
		catalog:     catalog,
		digest:      "abc",
		dropMissing: true,
		probeAll:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Models != 2 || result.Catalog != 3 || result.Probed != 3 {
		t.Fatalf("result=%+v", result)
	}
	if len(result.ProbeFailed) != 1 || result.ProbeFailed[0] != "anthropic/claude-opus-4-8" {
		t.Fatalf("failed=%v", result.ProbeFailed)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Ordinary.Routes) != 2 {
		t.Fatalf("routes=%+v", cfg.Ordinary.Routes)
	}
	for _, route := range cfg.Ordinary.Routes {
		if route.Provider == "anthropic" {
			t.Fatalf("anthropic should not be imported: %+v", route)
		}
	}
	if cfg.Tiers["strong"].Provider == "anthropic" {
		t.Fatalf("strong tier should not pin failed probe: %+v", cfg.Tiers)
	}
}

func TestSyncFromPiAllProbeFailuresLeaveConfigAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("QIANJI_CONFIG", cfgPath)

	existing := config.Default()
	existing.Ordinary.Routes = []config.Route{
		{ID: "aibase-kimi-k3", Circuit: "aibase:kimi-k3", Provider: "aibase", Model: "kimi-k3", Weight: 10},
	}
	if err := config.WriteAtomic(existing, "old"); err != nil {
		t.Fatal(err)
	}

	orig := catalogFilter
	t.Cleanup(func() { catalogFilter = orig })
	catalogFilter = func(catalog []config.CatalogItem, opts pi.FilterOpts) (pi.ProbeReport, error) {
		return pi.FilterCatalog(catalog, pi.FilterOpts{
			ProbeAll: true,
			Probe: func(config.CatalogItem) pi.ExecResult {
				return pi.ExecResult{OK: false, Error: "network down", ErrorType: "provider_failure"}
			},
		})
	}

	_, err := syncFromPi(syncOpts{
		reason:      "init",
		catalog:     []config.CatalogItem{{Provider: "aibase", Model: "kimi-k3"}},
		digest:      "new",
		dropMissing: true,
		probeAll:    true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "aibase-kimi-k3") {
		t.Fatalf("existing config should be unchanged:\n%s", raw)
	}
	if strings.Contains(string(raw), "pi_catalog_sha256 = \"new\"") {
		t.Fatal("failed probe should not rewrite catalog sha")
	}
}

func TestSyncFromPiDailySkipsCachedProbeFailures(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	t.Setenv("QIANJI_CONFIG", filepath.Join(dir, "config.toml"))

	existing := config.Default()
	existing.Ordinary.Routes = []config.Route{
		{ID: "aibase-kimi-k3", Circuit: "aibase:kimi-k3", Provider: "aibase", Model: "kimi-k3", Weight: 10},
	}
	if err := config.WriteAtomic(existing, "old"); err != nil {
		t.Fatal(err)
	}

	orig := catalogFilter
	t.Cleanup(func() { catalogFilter = orig })
	var probed []string
	catalogFilter = func(catalog []config.CatalogItem, opts pi.FilterOpts) (pi.ProbeReport, error) {
		return pi.FilterCatalog(catalog, pi.FilterOpts{
			ProbeAll:     opts.ProbeAll,
			Existing:     opts.Existing,
			CachedFailed: opts.CachedFailed,
			Probe: func(item config.CatalogItem) pi.ExecResult {
				probed = append(probed, pi.ItemRef(item.Provider, item.Model))
				return pi.ExecResult{OK: true, Output: "pong"}
			},
		})
	}

	// Seed probe_failed as if the previous init already excluded Anthropic.
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WithLock(cfg, true, func(st *state.State) error {
		st.PiSync.ProbeFailed = []string{"anthropic/claude-opus-4-8"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	result, err := syncFromPi(syncOpts{
		reason: "daily-pi-config-change",
		catalog: []config.CatalogItem{
			{Provider: "aibase", Model: "kimi-k3"},
			{Provider: "anthropic", Model: "claude-opus-4-8"},
			{Provider: "aibase", Model: "glm-5-2"},
		},
		digest:      "new",
		dropMissing: false,
		probeAll:    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(probed) != 1 || probed[0] != "aibase/glm-5-2" {
		t.Fatalf("probed=%v", probed)
	}
	if len(result.Added) != 1 || result.Added[0] != "aibase-glm-5-2" {
		t.Fatalf("added=%v", result.Added)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range loaded.Ordinary.Routes {
		if route.Provider == "anthropic" {
			t.Fatalf("cached failure re-imported: %+v", route)
		}
	}
}
