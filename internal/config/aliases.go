package config

import (
	"fmt"
	"strings"
)

var effortAliases = map[string]string{
	"":        "",
	"default": "",
	"off":     "off",
	"minimal": "minimal",
	"low":     "low",
	"medium":  "medium",
	"high":    "high",
	"xhigh":   "xhigh",
	"max":     "max",
}

var tierAliases = map[string]string{
	"ordinary":  "",
	"strong":    "strong",
	"强":         "strong",
	"强模型":       "strong",
	"main":      "strong",
	"strongest": "strongest",
	"最强":        "strongest",
	"最强模型":      "strongest",
	"important": "strongest",
}

func NormalizeEffort(value string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(value))
	out, ok := effortAliases[key]
	if !ok {
		return "", fmt.Errorf("unknown effort/effect: %s", value)
	}
	return out, nil
}

// NormalizeTier returns the canonical tier name, or empty string for ordinary pool.
func NormalizeTier(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	key := strings.TrimSpace(value)
	if out, ok := tierAliases[key]; ok {
		return out, nil
	}
	if out, ok := tierAliases[strings.ToLower(key)]; ok {
		return out, nil
	}
	return "", fmt.Errorf("unknown tier: %s", value)
}
