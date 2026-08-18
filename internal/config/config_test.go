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

func TestMergeKeepMissingRetainsStaleRoutes(t *testing.T) {
	existing := config.Default()
	existing.Ordinary.Routes = []config.Route{
		{ID: "acme-fast", Circuit: "acme:fast", Provider: "acme", Model: "fast", Weight: 25},
		{ID: "gone", Circuit: "old:gone", Provider: "old", Model: "gone", Weight: 9},
	}
	existing.Tiers = map[string]config.Tier{
		"strong":    {Provider: "acme", Model: "fast", Effort: "high"},
		"strongest": {Provider: "old", Model: "gone", Effort: "xhigh"},
	}
	catalog := []config.CatalogItem{
		{Provider: "acme", Model: "fast"},
		{Provider: "acme", Model: "pro"},
	}
	generated := config.GenerateFromCatalog(catalog, 10)
	merged, summary := config.MergeKeepMissing(existing, generated)
	byID := config.RouteByID(merged)
	if byID["acme-fast"].Weight != 25 {
		t.Fatalf("kept weight=%d", byID["acme-fast"].Weight)
	}
	if byID["acme-pro"].Weight != 1 {
		t.Fatalf("new weight=%d", byID["acme-pro"].Weight)
	}
	if byID["gone"].Weight != 9 {
		t.Fatalf("stale weight=%d", byID["gone"].Weight)
	}
	if len(summary.Removed) != 0 {
		t.Fatalf("removed=%v", summary.Removed)
	}
	if len(summary.Stale) != 1 || summary.Stale[0] != "gone" {
		t.Fatalf("stale=%v", summary.Stale)
	}
	if _, ok := merged.Tiers["strongest"]; !ok {
		t.Fatal("expected stale strongest tier kept")
	}
	if len(summary.DroppedTiers) != 0 {
		t.Fatalf("dropped_tiers=%v", summary.DroppedTiers)
	}
	if len(summary.StaleTiers) != 1 || summary.StaleTiers[0] != "strongest" {
		t.Fatalf("stale_tiers=%v", summary.StaleTiers)
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

func TestSuggestedTiersRanksByModelFamilyNotCatalogOrder(t *testing.T) {
	opus := config.SuggestedTiers([]config.CatalogItem{
		{Provider: "acme", Model: "fast"},
		{Provider: "anthropic", Model: "claude-opus-4-8"},
	})
	if opus["strong"].Model != "claude-opus-4-8" || opus["strongest"].Effort != "max" {
		t.Fatalf("%+v", opus)
	}

	sol := config.SuggestedTiers([]config.CatalogItem{
		{Provider: "aibase", Model: "deepseek-v4-flash"},
		{Provider: "aibase", Model: "deepseek-v4-pro"},
		{Provider: "onehz", Model: "gpt-5.6-luna"},
		{Provider: "onehz", Model: "gpt-5.6-sol"},
	})
	if sol["strong"].Provider != "onehz" || sol["strong"].Model != "gpt-5.6-sol" {
		t.Fatalf("expected gpt-5.6-sol over deepseek, got %+v", sol)
	}
	if sol["strongest"].Effort != "xhigh" {
		t.Fatalf("expected xhigh for gpt-5.6, got %+v", sol["strongest"])
	}

	tied := config.SuggestedTiers([]config.CatalogItem{
		{Provider: "acme", Model: "foo"},
		{Provider: "acme", Model: "bar"},
	})
	if tied["strong"].Model != "foo" {
		t.Fatalf("equal unknown models should keep catalog order: %+v", tied)
	}
}

func TestStrengthScoreFlashBelowProAndGPT(t *testing.T) {
	flash := config.StrengthScore("aibase", "deepseek-v4-flash")
	pro := config.StrengthScore("aibase", "deepseek-v4-pro")
	sol := config.StrengthScore("onehz", "gpt-5.6-sol")
	if !(sol > pro && pro > flash) {
		t.Fatalf("sol=%d pro=%d flash=%d", sol, pro, flash)
	}
}

func TestMergeInitRetargetsCanonicalTiers(t *testing.T) {
	existing := config.Default()
	existing.Ordinary.Routes = []config.Route{
		{ID: "aibase-deepseek-v4-flash", Circuit: "aibase:deepseek-v4-flash", Provider: "aibase", Model: "deepseek-v4-flash", Weight: 10},
		{ID: "onehz-gpt-5.6-sol", Circuit: "onehz:gpt-5.6-sol", Provider: "onehz", Model: "gpt-5.6-sol", Weight: 10},
	}
	existing.Tiers = map[string]config.Tier{
		"strong": {Provider: "aibase", Model: "deepseek-v4-flash", Effort: "high"},
	}
	generated := config.GenerateFromCatalog([]config.CatalogItem{
		{Provider: "aibase", Model: "deepseek-v4-flash"},
		{Provider: "onehz", Model: "gpt-5.6-sol"},
	}, 10)
	merged, _ := config.Merge(existing, generated)
	if merged.Tiers["strong"].Model != "gpt-5.6-sol" {
		t.Fatalf("init should retarget strong: %+v", merged.Tiers)
	}
	if merged.Ordinary.Routes[0].Weight != 10 || merged.Ordinary.Routes[1].Weight != 10 {
		t.Fatalf("weights should be kept: %+v", merged.Ordinary.Routes)
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
