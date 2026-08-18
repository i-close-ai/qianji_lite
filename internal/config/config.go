package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const Version = 1

type Config struct {
	Executor Executor        `toml:"executor"`
	Strategy Strategy        `toml:"strategy"`
	Tiers    map[string]Tier `toml:"tiers"`
	Ordinary Ordinary        `toml:"ordinary"`
}

type Executor struct {
	Backend string `toml:"backend" json:"backend"`
	Command string `toml:"command" json:"command"`
}

type Strategy struct {
	Ordinary           string  `toml:"ordinary" json:"ordinary"`
	StickyProbability  float64 `toml:"sticky_probability" json:"sticky_probability"`
	AffinityTTLSeconds int     `toml:"affinity_ttl_seconds" json:"affinity_ttl_seconds"`
}

type Tier struct {
	Provider string `toml:"provider" json:"provider"`
	Model    string `toml:"model" json:"model"`
	Effort   string `toml:"effort" json:"effort"`
	Effect   string `toml:"effect,omitempty" json:"effect,omitempty"`
}

type Ordinary struct {
	Routes []Route `toml:"routes"`
}

type Route struct {
	ID       string `toml:"id" json:"id"`
	Circuit  string `toml:"circuit" json:"circuit"`
	Provider string `toml:"provider" json:"provider"`
	Model    string `toml:"model" json:"model"`
	Effort   string `toml:"effort" json:"effort"`
	Weight   int    `toml:"weight" json:"weight"`
	Via      string `toml:"-" json:"via,omitempty"`
}

func Default() Config {
	return Config{
		Executor: Executor{Backend: "pi", Command: "pi"},
		Strategy: Strategy{
			Ordinary:           "weighted_random",
			StickyProbability:  0.85,
			AffinityTTLSeconds: 604800,
		},
		Tiers:    map[string]Tier{},
		Ordinary: Ordinary{Routes: []Route{}},
	}
}

func Home() string {
	if v := strings.TrimSpace(os.Getenv("QIANJI_HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".qianji")
	}
	return filepath.Join(home, ".qianji")
}

func Path() string {
	if v := strings.TrimSpace(os.Getenv("QIANJI_CONFIG")); v != "" {
		return v
	}
	return filepath.Join(Home(), "config.toml")
}

func StatePath() string {
	return filepath.Join(Home(), "state.json")
}

func LockPath() string {
	return filepath.Join(Home(), ".lock")
}

func PiAgentHome() string {
	if v := strings.TrimSpace(os.Getenv("PI_AGENT_HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".pi", "agent")
	}
	return filepath.Join(home, ".pi", "agent")
}

func Load() (Config, error) {
	cfg := Default()
	path := Path()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return normalize(cfg), nil
		}
		return Config{}, err
	}
	var parsed Config
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if parsed.Executor.Backend != "" || parsed.Executor.Command != "" {
		if parsed.Executor.Backend != "" {
			cfg.Executor.Backend = parsed.Executor.Backend
		}
		if parsed.Executor.Command != "" {
			cfg.Executor.Command = parsed.Executor.Command
		}
	}
	if parsed.Tiers != nil {
		if cfg.Tiers == nil {
			cfg.Tiers = map[string]Tier{}
		}
		for name, tier := range parsed.Tiers {
			cfg.Tiers[name] = tier
		}
	}
	if parsed.Strategy.Ordinary != "" {
		cfg.Strategy.Ordinary = parsed.Strategy.Ordinary
	}
	if parsed.Strategy.StickyProbability != 0 {
		cfg.Strategy.StickyProbability = parsed.Strategy.StickyProbability
	}
	if parsed.Strategy.AffinityTTLSeconds != 0 {
		cfg.Strategy.AffinityTTLSeconds = parsed.Strategy.AffinityTTLSeconds
	}
	if parsed.Ordinary.Routes != nil {
		cfg.Ordinary.Routes = parsed.Ordinary.Routes
	}
	return normalize(cfg), nil
}

func MustLoad() Config {
	cfg, err := Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "qianji: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func normalize(cfg Config) Config {
	sticky := cfg.Strategy.StickyProbability
	if sticky == 0 {
		sticky = 0.85
	}
	if sticky < 0 {
		sticky = 0
	}
	if sticky > 1 {
		sticky = 1
	}
	cfg.Strategy.StickyProbability = sticky
	if cfg.Strategy.AffinityTTLSeconds == 0 {
		cfg.Strategy.AffinityTTLSeconds = 604800
	}
	if cfg.Strategy.Ordinary == "" {
		cfg.Strategy.Ordinary = "weighted_random"
	}
	if cfg.Tiers == nil {
		cfg.Tiers = map[string]Tier{}
	}
	routes := make([]Route, 0, len(cfg.Ordinary.Routes))
	for _, route := range cfg.Ordinary.Routes {
		item := route
		item.ID = strings.TrimSpace(item.ID)
		item.Provider = strings.TrimSpace(item.Provider)
		item.Model = strings.TrimSpace(item.Model)
		effort, err := NormalizeEffort(firstNonEmpty(item.Effort, route.Effort))
		if err != nil {
			fmt.Fprintf(os.Stderr, "qianji: %v\n", err)
			os.Exit(1)
		}
		item.Effort = effort
		item.Circuit = strings.TrimSpace(item.Circuit)
		if item.Circuit == "" {
			item.Circuit = item.Provider + ":" + item.Model
		}
		routes = append(routes, item)
	}
	cfg.Ordinary.Routes = routes
	if cfg.Executor.Backend == "" {
		cfg.Executor.Backend = "pi"
	}
	if cfg.Executor.Command == "" {
		cfg.Executor.Command = "pi"
	}
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func RouteByID(cfg Config) map[string]Route {
	out := make(map[string]Route, len(cfg.Ordinary.Routes))
	for _, route := range cfg.Ordinary.Routes {
		out[route.ID] = route
	}
	return out
}

func CircuitIDs(cfg Config) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, route := range cfg.Ordinary.Routes {
		if _, ok := seen[route.Circuit]; ok {
			continue
		}
		seen[route.Circuit] = struct{}{}
		ids = append(ids, route.Circuit)
	}
	return ids
}

func DefaultRouteID(provider, model string) string {
	slug := strings.ReplaceAll(model, "/", "-")
	slug = strings.Trim(slug, "-")
	return provider + "-" + slug
}

func RouteKey(provider, model string) string {
	return provider + "\x00" + model
}

func ParseModelRef(value string) (provider, model string) {
	if strings.Contains(value, "/") {
		provider, model, _ = strings.Cut(value, "/")
		return provider, model
	}
	return "", value
}

func tomlQuote(value string) string {
	b, _ := json.Marshal(value)
	return string(b)
}

func Render(cfg Config, sha256 string) string {
	var b strings.Builder
	b.WriteString("# Qianji routing policy. Generated from `pi --list-models` (custom + official providers).\n")
	b.WriteString("# Pi owns providers, models, and API keys. Qianji only owns weights, tiers, and circuits.\n")
	b.WriteString("# pi_catalog_sha256 = " + sha256 + "\n")
	b.WriteString("# Re-run `qianji init` or `qianji reinit` after Pi models/auth change. First command each day also checks.\n")
	b.WriteString("\n")
	b.WriteString("version = 1\n\n")
	b.WriteString("[executor]\n")
	b.WriteString("backend = " + tomlQuote(orDefault(cfg.Executor.Backend, "pi")) + "\n")
	b.WriteString("command = " + tomlQuote(orDefault(cfg.Executor.Command, "pi")) + "\n\n")
	b.WriteString("[strategy]\n")
	b.WriteString("ordinary = " + tomlQuote(orDefault(cfg.Strategy.Ordinary, "weighted_random")) + "\n")
	b.WriteString(fmt.Sprintf("sticky_probability = %v\n", cfg.Strategy.StickyProbability))
	b.WriteString(fmt.Sprintf("affinity_ttl_seconds = %d\n\n", cfg.Strategy.AffinityTTLSeconds))

	// Stable-ish order: strong, strongest, then aliases, then others.
	order := []string{"strong", "strongest", "main", "important"}
	seen := map[string]struct{}{}
	writeTier := func(name string, tier Tier) {
		if name == "strong" || name == "main" {
			if name == "strong" {
				b.WriteString("# 「使用qianji强模型」\n")
			} else {
				b.WriteString("# alias of strong\n")
			}
		} else if name == "strongest" || name == "important" {
			if name == "strongest" {
				b.WriteString("# 「使用qianji最强模型」\n")
			} else {
				b.WriteString("# alias of strongest\n")
			}
		}
		b.WriteString("[tiers." + name + "]\n")
		b.WriteString("provider = " + tomlQuote(tier.Provider) + "\n")
		b.WriteString("model = " + tomlQuote(tier.Model) + "\n")
		b.WriteString("effort = " + tomlQuote(tier.Effort) + "\n\n")
		seen[name] = struct{}{}
	}
	for _, name := range order {
		if tier, ok := cfg.Tiers[name]; ok {
			writeTier(name, tier)
		}
	}
	var extras []string
	for name := range cfg.Tiers {
		if _, ok := seen[name]; !ok {
			extras = append(extras, name)
		}
	}
	for _, name := range sorted(extras) {
		writeTier(name, cfg.Tiers[name])
	}

	b.WriteString("# 「使用qianji」→ ordinary pool. Weights are relative; they need not sum to 100.\n")
	b.WriteString("# New models imported from Pi start at weight = 1 until you raise them.\n\n")
	for _, route := range cfg.Ordinary.Routes {
		b.WriteString("[[ordinary.routes]]\n")
		b.WriteString("id = " + tomlQuote(route.ID) + "\n")
		b.WriteString("circuit = " + tomlQuote(route.Circuit) + "\n")
		b.WriteString("provider = " + tomlQuote(route.Provider) + "\n")
		b.WriteString("model = " + tomlQuote(route.Model) + "\n")
		b.WriteString("effort = " + tomlQuote(route.Effort) + "\n")
		b.WriteString(fmt.Sprintf("weight = %d\n\n", route.Weight))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func WriteAtomic(cfg Config, sha256 string) error {
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	text := Render(cfg, sha256)
	tmp, err := os.CreateTemp(Home(), "qianji-config-*.toml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, Path())
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func sorted(items []string) []string {
	out := append([]string(nil), items...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
