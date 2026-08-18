package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type CatalogItem struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type MergeSummary struct {
	Added        []string `json:"added"`
	Kept         []string `json:"kept"`
	Removed      []string `json:"removed"`
	DroppedTiers []string `json:"dropped_tiers"`
	Routes       int      `json:"routes"`
}

func SuggestedTiers(catalog []CatalogItem) map[string]Tier {
	available := map[string]struct{}{}
	for _, item := range catalog {
		available[RouteKey(item.Provider, item.Model)] = struct{}{}
	}
	candidates := []struct {
		provider, model, high, strongest string
	}{
		{"anthropic", "claude-opus-4-8", "high", "max"},
		{"anthropic", "claude-opus-4-7", "high", "max"},
		{"openai", "gpt-5.6", "high", "xhigh"},
		{"openai", "gpt-5", "high", "high"},
	}
	tiers := map[string]Tier{}
	for _, c := range candidates {
		if _, ok := available[RouteKey(c.provider, c.model)]; ok {
			tiers["strong"] = Tier{Provider: c.provider, Model: c.model, Effort: c.high}
			tiers["strongest"] = Tier{Provider: c.provider, Model: c.model, Effort: c.strongest}
			tiers["main"] = tiers["strong"]
			tiers["important"] = tiers["strongest"]
			return tiers
		}
	}
	settings := loadPiSettings()
	provider := strings.TrimSpace(asString(settings["defaultProvider"]))
	model := strings.TrimSpace(asString(settings["defaultModel"]))
	if provider != "" && model != "" {
		if _, ok := available[RouteKey(provider, model)]; ok {
			effort := strings.TrimSpace(asString(settings["defaultThinkingLevel"]))
			if effort == "" {
				effort = "high"
			}
			norm, err := NormalizeEffort(effort)
			if err != nil || norm == "" {
				norm = "high"
			}
			strongest := "xhigh"
			if norm == "xhigh" {
				strongest = norm
			}
			tiers["strong"] = Tier{Provider: provider, Model: model, Effort: norm}
			tiers["strongest"] = Tier{Provider: provider, Model: model, Effort: strongest}
			return tiers
		}
	}
	if len(catalog) > 0 {
		item := catalog[0]
		tiers["strong"] = Tier{Provider: item.Provider, Model: item.Model, Effort: "high"}
		tiers["strongest"] = Tier{Provider: item.Provider, Model: item.Model, Effort: "xhigh"}
	}
	return tiers
}

func GenerateFromCatalog(catalog []CatalogItem, newRouteWeight int) Config {
	cfg := Default()
	cfg.Tiers = SuggestedTiers(catalog)
	routes := make([]Route, 0, len(catalog))
	for _, item := range catalog {
		routes = append(routes, Route{
			ID:       DefaultRouteID(item.Provider, item.Model),
			Circuit:  item.Provider + ":" + item.Model,
			Provider: item.Provider,
			Model:    item.Model,
			Effort:   "",
			Weight:   newRouteWeight,
		})
	}
	cfg.Ordinary.Routes = routes
	return cfg
}

func Merge(existing, generated Config) (Config, MergeSummary) {
	merged := Default()
	if existing.Executor.Backend != "" || existing.Executor.Command != "" {
		merged.Executor = existing.Executor
	} else {
		merged.Executor = generated.Executor
	}
	merged.Strategy = generated.Strategy
	if existing.Strategy.Ordinary != "" {
		merged.Strategy.Ordinary = existing.Strategy.Ordinary
	}
	if existing.Strategy.StickyProbability != 0 {
		merged.Strategy.StickyProbability = existing.Strategy.StickyProbability
	}
	if existing.Strategy.AffinityTTLSeconds != 0 {
		merged.Strategy.AffinityTTLSeconds = existing.Strategy.AffinityTTLSeconds
	}

	available := map[string]struct{}{}
	for _, route := range generated.Ordinary.Routes {
		available[RouteKey(route.Provider, route.Model)] = struct{}{}
	}

	tiers := map[string]Tier{}
	var dropped []string
	for name, tier := range existing.Tiers {
		key := RouteKey(strings.TrimSpace(tier.Provider), strings.TrimSpace(tier.Model))
		if _, ok := available[key]; ok {
			effort, _ := NormalizeEffort(firstNonEmpty(tier.Effort, tier.Effect))
			tier.Effort = effort
			tier.Effect = ""
			tiers[name] = tier
		} else {
			dropped = append(dropped, name)
		}
	}
	for name, tier := range generated.Tiers {
		if _, ok := tiers[name]; !ok {
			tiers[name] = tier
		}
	}
	merged.Tiers = tiers

	existingByKey := map[string]Route{}
	for _, route := range existing.Ordinary.Routes {
		existingByKey[RouteKey(route.Provider, route.Model)] = route
	}
	hadRoutes := len(existing.Ordinary.Routes) > 0
	var routes []Route
	var added, kept []string
	for _, route := range generated.Ordinary.Routes {
		key := RouteKey(route.Provider, route.Model)
		if old, ok := existingByKey[key]; ok {
			item := route
			if strings.TrimSpace(old.ID) != "" {
				item.ID = old.ID
			}
			if strings.TrimSpace(old.Circuit) != "" {
				item.Circuit = old.Circuit
			}
			effort, _ := NormalizeEffort(firstNonEmpty(old.Effort, route.Effort))
			item.Effort = effort
			item.Weight = old.Weight
			routes = append(routes, item)
			kept = append(kept, item.ID)
		} else {
			item := route
			if hadRoutes {
				item.Weight = 1
			}
			routes = append(routes, item)
			added = append(added, item.ID)
		}
	}
	var removed []string
	for _, route := range existing.Ordinary.Routes {
		if _, ok := available[RouteKey(route.Provider, route.Model)]; !ok {
			id := route.ID
			if id == "" {
				id = route.Provider + "/" + route.Model
			}
			removed = append(removed, id)
		}
	}
	merged.Ordinary.Routes = routes
	return merged, MergeSummary{
		Added:        added,
		Kept:         kept,
		Removed:      removed,
		DroppedTiers: dropped,
		Routes:       len(routes),
	}
}

func loadPiSettings() map[string]any {
	path := filepath.Join(PiAgentHome(), "settings.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return map[string]any{}
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
