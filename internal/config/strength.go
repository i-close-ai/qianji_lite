package config

import (
	"strings"
	"unicode"
)

func SuggestedTiers(catalog []CatalogItem) map[string]Tier {
	tiers := map[string]Tier{}
	item, ok := pickStrongest(catalog)
	if !ok {
		return tiers
	}
	high, maxEffort := suggestedEfforts(item.Model)
	tiers["strong"] = Tier{Provider: item.Provider, Model: item.Model, Effort: high}
	tiers["strongest"] = Tier{Provider: item.Provider, Model: item.Model, Effort: maxEffort}
	tiers["main"] = tiers["strong"]
	tiers["important"] = tiers["strongest"]
	return tiers
}

func pickStrongest(catalog []CatalogItem) (CatalogItem, bool) {
	if len(catalog) == 0 {
		return CatalogItem{}, false
	}
	best := catalog[0]
	bestScore := StrengthScore(best.Provider, best.Model)
	for _, item := range catalog[1:] {
		score := StrengthScore(item.Provider, item.Model)
		if score > bestScore {
			best = item
			bestScore = score
		}
	}
	return best, true
}

func suggestedEfforts(model string) (high, strongest string) {
	if looksLikeOpus(model) {
		return "high", "max"
	}
	return "high", "xhigh"
}

func looksLikeOpus(model string) bool {
	return hasToken(strings.ToLower(model), "opus")
}

// StrengthScore ranks a catalog model for 强/最强 pinning. Provider is ignored:
// onehz/gpt-5.6-sol should outrank aibase/deepseek-v4-flash.
func StrengthScore(provider, model string) int {
	_ = provider
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return 0
	}
	return familyScore(m) + variantScore(m) + versionBonus(m)
}

func familyScore(m string) int {
	switch {
	case strings.Contains(m, "opus"):
		return 90000
	case strings.Contains(m, "gpt-5.6") || strings.Contains(m, "gpt5.6"):
		return 85000
	case strings.Contains(m, "gpt-5.5") || strings.Contains(m, "gpt5.5"):
		return 82000
	case strings.Contains(m, "sonnet"):
		return 80000
	case strings.Contains(m, "gpt-5.4") || strings.Contains(m, "gpt5.4"):
		return 78000
	case strings.Contains(m, "gpt-5.3") || strings.Contains(m, "gpt5.3"):
		return 76000
	case strings.Contains(m, "gpt-5") || strings.Contains(m, "gpt5"):
		return 74000
	case strings.Contains(m, "gemini-3") || strings.Contains(m, "gemini-2.5-pro"):
		return 73000
	case strings.Contains(m, "kimi"):
		return 62000
	case strings.Contains(m, "qwen"):
		return 61000
	case strings.Contains(m, "glm"):
		return 60000
	case strings.Contains(m, "deepseek"):
		return 55000
	case strings.Contains(m, "haiku"):
		return 40000
	default:
		return 20000
	}
}

func variantScore(m string) int {
	score := 0
	switch {
	case hasToken(m, "flash"), hasToken(m, "mini"), hasToken(m, "nano"),
		hasToken(m, "lite"), hasToken(m, "small"), hasToken(m, "haiku"),
		hasToken(m, "fast"):
		score -= 25000
	}
	if hasToken(m, "pro") {
		score += 800
	}
	if hasToken(m, "max") {
		score += 700
	}
	if hasToken(m, "codex") {
		score += 300
	}
	if hasToken(m, "sol") {
		score += 400
	}
	if hasToken(m, "terra") {
		score += 250
	}
	if hasToken(m, "luna") {
		score += 100
	}
	return score
}

func versionBonus(m string) int {
	n := 0
	digits := 0
	for _, r := range m {
		if unicode.IsDigit(r) {
			n = n*10 + int(r-'0')
			digits++
			if digits >= 4 {
				break
			}
		}
	}
	return n
}

func hasToken(model, tok string) bool {
	start := 0
	for i := 0; i <= len(model); i++ {
		if i < len(model) && isTokenChar(model[i]) {
			continue
		}
		if i > start && strings.EqualFold(model[start:i], tok) {
			return true
		}
		start = i + 1
	}
	return false
}

func isTokenChar(b byte) bool {
	return b != '-' && b != '_' && b != '.' && b != '/' && b != ' '
}

func isCanonicalTier(name string) bool {
	switch name {
	case "strong", "strongest", "main", "important":
		return true
	default:
		return false
	}
}
