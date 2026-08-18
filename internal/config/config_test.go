package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/i-close-ai/qianji_lite/internal/config"
)

func TestNormalizeEffortAndTier(t *testing.T) {
	effort, err := config.NormalizeEffort("xhigh")
	if err != nil || effort != "xhigh" {
		t.Fatalf("effort: %q %v", effort, err)
	}
	tier, err := config.NormalizeTier("强模型")
	if err != nil || tier != "strong" {
		t.Fatalf("tier: %q %v", tier, err)
	}
	tier, err = config.NormalizeTier("important")
	if err != nil || tier != "strongest" {
		t.Fatalf("alias: %q %v", tier, err)
	}
	tier, err = config.NormalizeTier("ordinary")
	if err != nil || tier != "" {
		t.Fatalf("ordinary: %q %v", tier, err)
	}
}

func TestMergeKeepsWeightsAddsNewAtOne(t *testing.T) {
	existing := config.Default()
	existing.Ordinary.Routes = []config.Route{
		{ID: "acme-fast", Circuit: "acme:fast", Provider: "acme", Model: "fast", Weight: 25},
		{ID: "gone", Circuit: "old:gone", Provider: "old", Model: "gone", Weight: 9},
	}
	existing.Tiers = map[string]config.Tier{
		"strong": {Provider: "acme", Model: "fast", Effort: "high"},
	}
	catalog := []config.CatalogItem{
		{Provider: "acme", Model: "fast"},
		{Provider: "acme", Model: "pro"},
	}
	generated := config.GenerateFromCatalog(catalog, 10)
	merged, summary := config.Merge(existing, generated)
	if len(merged.Ordinary.Routes) != 2 {
		t.Fatalf("routes=%d", len(merged.Ordinary.Routes))
	}
	byID := config.RouteByID(merged)
	if byID["acme-fast"].Weight != 25 {
		t.Fatalf("kept weight=%d", byID["acme-fast"].Weight)
	}
	if byID["acme-pro"].Weight != 1 {
		t.Fatalf("new weight=%d", byID["acme-pro"].Weight)
	}
	if len(summary.Removed) != 1 || summary.Removed[0] != "gone" {
		t.Fatalf("removed=%v", summary.Removed)
	}
	if _, ok := merged.Tiers["strong"]; !ok {
		t.Fatal("expected strong tier kept")
	}
}

func TestRenderRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	t.Setenv("QIANJI_CONFIG", filepath.Join(dir, "config.toml"))
	cfg := config.Default()
	cfg.Ordinary.Routes = []config.Route{
		{ID: "acme-fast", Circuit: "acme:fast", Provider: "acme", Model: "fast", Weight: 25},
	}
	cfg.Tiers["strong"] = config.Tier{Provider: "acme", Model: "fast", Effort: "high"}
	if err := config.WriteAtomic(cfg, "abc"); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Ordinary.Routes) != 1 || loaded.Ordinary.Routes[0].Weight != 25 {
		t.Fatalf("%+v", loaded.Ordinary.Routes)
	}
	if loaded.Tiers["strong"].Model != "fast" {
		t.Fatalf("%+v", loaded.Tiers)
	}
}

func TestSuggestedTiersPrefersPublicCatalogThenFirst(t *testing.T) {
	t.Setenv("PI_AGENT_HOME", t.TempDir())
	opus := config.SuggestedTiers([]config.CatalogItem{
		{Provider: "acme", Model: "fast"},
		{Provider: "anthropic", Model: "claude-opus-4-8"},
	})
	if opus["strong"].Model != "claude-opus-4-8" || opus["strongest"].Effort != "max" {
		t.Fatalf("%+v", opus)
	}
	first := config.SuggestedTiers([]config.CatalogItem{
		{Provider: "acme", Model: "fast"},
	})
	if first["strong"].Provider != "acme" || first["strong"].Model != "fast" {
		t.Fatalf("%+v", first)
	}
}

func TestHomeOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QIANJI_HOME", dir)
	if config.Home() != dir {
		t.Fatalf("home=%s", config.Home())
	}
	_ = os.MkdirAll(dir, 0o755)
}
