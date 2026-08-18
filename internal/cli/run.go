package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/pi"
	"github.com/i-close-ai/qianji_lite/internal/router"
	"github.com/i-close-ai/qianji_lite/internal/state"
)

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prompt := fs.String("prompt", "", "")
	promptFile := fs.String("prompt-file", "", "")
	workdir := fs.String("workdir", "", "")
	timeout := fs.Int("timeout", 600, "")
	maxAttempts := fs.Int("max-attempts", 8, "")
	routeID := fs.String("route", "", "")
	tier := fs.String("tier", "", "")
	provider := fs.String("provider", "", "")
	model := fs.String("model", "", "")
	effort := fs.String("effort", "", "")
	effect := fs.String("effect", "", "")
	affinityKey := fs.String("affinity-key", "", "")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	cfg := config.MustLoad()
	text, err := readPrompt(*prompt, *promptFile)
	if err != nil {
		return fail(err)
	}
	tierName := *tier
	if tierName != "" {
		tierName, err = config.NormalizeTier(tierName)
		if err != nil {
			return fail(err)
		}
	}
	explicit := *routeID != "" || tierName != "" || *provider != "" || *model != ""
	if explicit {
		route, err := resolveExplicit(cfg, *routeID, tierName, *provider, *model, *effort, *effect)
		if err != nil {
			return fail(err)
		}
		printRoute(route)
		res := pi.RunRoute(cfg, route, text, *workdir, *timeout)
		if res.OK {
			writeOutput(res.Output)
			if _, ok := config.RouteByID(cfg)[route.ID]; ok {
				_ = state.WithLock(cfg, true, func(st *state.State) error {
					router.MarkSuccess(cfg, st, route.ID, -1)
					return nil
				})
			}
			return 0
		}
		fmt.Fprintf(os.Stderr, "qianji %s: %s (%s)\n", orDefault(res.ErrorType, "failure"), route.ID, res.Error)
		if _, ok := config.RouteByID(cfg)[route.ID]; ok && res.ErrorType == "provider_failure" {
			_ = state.WithLock(cfg, true, func(st *state.State) error {
				router.MarkFailure(cfg, st, route.ID, res.Error, -1)
				return nil
			})
		}
		if res.ErrorType == "timeout" {
			return 2
		}
		return 1
	}

	affinityHash := router.ResolveAffinityHash(*affinityKey, text, *workdir)
	attempted := []string{}
	consecutiveTimeouts := 0
	exclude := map[string]struct{}{}
	attempts := *maxAttempts
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		var route *config.Route
		var earliest int64
		err := state.WithLock(cfg, true, func(st *state.State) error {
			route = router.Select(cfg, st, -1, affinityHash, exclude, nil)
			if route == nil {
				earliest = router.EarliestBlocked(st)
			}
			return nil
		})
		if err != nil {
			return fail(err)
		}
		if route == nil {
			fmt.Fprintf(os.Stderr, "qianji: all ordinary routes blocked until %s\n", router.FormatTime(earliest))
			return 75
		}
		chosen := *route
		attempted = append(attempted, chosen.ID)
		printRoute(chosen)
		res := pi.RunRoute(cfg, chosen, text, *workdir, *timeout)
		if res.OK {
			_ = state.WithLock(cfg, true, func(st *state.State) error {
				router.MarkSuccess(cfg, st, chosen.ID, -1)
				router.RememberAffinity(st, affinityHash, chosen.ID, -1)
				return nil
			})
			writeOutput(res.Output)
			return 0
		}
		if res.ErrorType == "timeout" {
			consecutiveTimeouts++
			exclude[chosen.ID] = struct{}{}
			_ = state.WithLock(cfg, true, func(st *state.State) error {
				router.ClearAffinity(st, affinityHash)
				return nil
			})
			fmt.Fprintf(os.Stderr, "qianji timeout: %s — task too large (%s), retrying different model\n", chosen.ID, res.Error)
			if consecutiveTimeouts >= 2 {
				fmt.Fprintf(os.Stderr, "qianji: task too large — %d consecutive timeouts (%s). Split the task into smaller units and retry.\n",
					consecutiveTimeouts, strings.Join(attempted, ", "))
				return 2
			}
			continue
		}
		consecutiveTimeouts = 0
		exclude[chosen.ID] = struct{}{}
		var delay int64
		var blockedUntil int64
		_ = state.WithLock(cfg, true, func(st *state.State) error {
			delay = router.MarkFailure(cfg, st, chosen.ID, res.Error, -1)
			router.ClearAffinity(st, affinityHash)
			blockedUntil = st.Circuits[chosen.Circuit].BlockedUntil
			return nil
		})
		fmt.Fprintf(os.Stderr, "qianji failure: %s -> blocked %ds until %s (%s)\n",
			chosen.ID, delay, router.FormatTime(blockedUntil), res.Error)
	}
	fmt.Fprintf(os.Stderr, "qianji: exhausted %d attempts: %s\n", len(attempted), strings.Join(attempted, ", "))
	return 1
}

func readPrompt(prompt, promptFile string) (string, error) {
	if promptFile != "" {
		raw, err := os.ReadFile(promptFile)
		if err != nil {
			return "", err
		}
		prompt = string(raw)
	}
	if prompt == "" {
		stat, _ := os.Stdin.Stat()
		if stat != nil && stat.Mode()&os.ModeCharDevice == 0 {
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", err
			}
			prompt = string(raw)
		}
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt is required via --prompt, --prompt-file, or stdin")
	}
	return prompt, nil
}

func resolveExplicit(cfg config.Config, routeID, tierName, provider, model, effort, effect string) (config.Route, error) {
	lookup := config.RouteByID(cfg)
	var route config.Route
	switch {
	case routeID != "":
		r, ok := lookup[routeID]
		if !ok {
			return config.Route{}, fmt.Errorf("unknown route: %s", routeID)
		}
		route = r
	case tierName != "":
		tier, ok := cfg.Tiers[tierName]
		if !ok {
			return config.Route{}, fmt.Errorf("unknown tier: %s", tierName)
		}
		effortVal, err := config.NormalizeEffort(first(tier.Effort, tier.Effect))
		if err != nil {
			return config.Route{}, err
		}
		route = config.Route{
			ID:       "tier-" + tierName,
			Circuit:  tier.Provider + ":" + tier.Model,
			Provider: tier.Provider,
			Model:    tier.Model,
			Effort:   effortVal,
			Weight:   0,
		}
	default:
		if model != "" && strings.Contains(model, "/") && provider == "" {
			provider, model = config.ParseModelRef(model)
		}
		if provider == "" || model == "" {
			return config.Route{}, fmt.Errorf("explicit run requires --route, --tier, or --provider plus --model")
		}
		route = config.Route{
			ID:       provider + "-" + model,
			Circuit:  provider + ":" + model,
			Provider: provider,
			Model:    model,
			Effort:   "",
			Weight:   0,
		}
	}
	override := effort
	if override == "" {
		override = effect
	}
	if override != "" {
		val, err := config.NormalizeEffort(override)
		if err != nil {
			return config.Route{}, err
		}
		route.Effort = val
	}
	return route, nil
}

func printRoute(route config.Route) {
	extra := ""
	if route.Via != "" {
		extra = " via=" + route.Via
	}
	effort := route.Effort
	if effort == "" {
		effort = "default"
	}
	fmt.Fprintf(os.Stderr, "qianji route: %s provider=%s model=%s effort=%s%s\n",
		route.ID, route.Provider, route.Model, effort, extra)
}

func writeOutput(output string) {
	os.Stdout.WriteString(output)
	if output != "" && !strings.HasSuffix(output, "\n") {
		os.Stdout.WriteString("\n")
	}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func cmdChoose(args []string) int {
	fs := flag.NewFlagSet("choose", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "")
	affinityKey := fs.String("affinity-key", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg := config.MustLoad()
	affinityHash := router.ResolveAffinityHash(*affinityKey, "", "")
	var selected *config.Route
	var selectedState state.Counters
	var earliest int64
	err := state.WithLock(cfg, true, func(st *state.State) error {
		selected = router.Select(cfg, st, -1, affinityHash, nil, nil)
		if selected == nil {
			earliest = router.EarliestBlocked(st)
			return nil
		}
		selectedState = st.Routes[selected.ID]
		return nil
	})
	if err != nil {
		return fail(err)
	}
	if selected == nil {
		if *asJSON {
			printJSON(map[string]any{"ok": false, "reason": "all_routes_temporarily_blocked", "retry_at": earliest})
		} else {
			fmt.Printf("No route available until %s\n", router.FormatTime(earliest))
		}
		return 75
	}
	if *asJSON {
		out := map[string]any{
			"id":       selected.ID,
			"circuit":  selected.Circuit,
			"provider": selected.Provider,
			"model":    selected.Model,
			"effort":   selected.Effort,
			"weight":   selected.Weight,
			"via":      selected.Via,
			"state":    selectedState,
		}
		raw, _ := json.Marshal(out)
		fmt.Println(string(raw))
		return 0
	}
	via := selected.Via
	if via == "" {
		via = "weighted_random"
	}
	effort := selected.Effort
	if effort == "" {
		effort = "default"
	}
	fmt.Printf("%s provider=%s model=%s effort=%s via=%s\n", selected.ID, selected.Provider, selected.Model, effort, via)
	return 0
}

func cmdStatus(args []string) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	cfg := config.MustLoad()
	now := time.Now().Unix()
	type row struct {
		config.Route
		Status              string `json:"status"`
		BlockedUntil        int64  `json:"blocked_until"`
		ConsecutiveFailures int    `json:"consecutive_failures"`
		Selections          int    `json:"selections"`
		Successes           int    `json:"successes"`
		Failures            int    `json:"failures"`
		LastError           string `json:"last_error"`
	}
	var rows []row
	var affinityN int
	var sync state.PiSync
	_ = state.WithLock(cfg, false, func(st *state.State) error {
		affinityN = len(st.Affinity)
		sync = st.PiSync
		for _, route := range cfg.Ordinary.Routes {
			rs := st.Routes[route.ID]
			cs := st.Circuits[route.Circuit]
			status := "ready"
			if cs.BlockedUntil > now {
				status = "blocked"
			}
			rows = append(rows, row{
				Route:               route,
				Status:              status,
				BlockedUntil:        cs.BlockedUntil,
				ConsecutiveFailures: cs.ConsecutiveFailures,
				Selections:          rs.Selections,
				Successes:           rs.Successes,
				Failures:            rs.Failures,
				LastError:           cs.LastError,
			})
		}
		return nil
	})
	if asJSON {
		printJSON(map[string]any{
			"now":              now,
			"strategy":         cfg.Strategy,
			"affinity_entries": affinityN,
			"pi_sync":          sync,
			"routes":           rows,
			"tiers":            cfg.Tiers,
		})
		return 0
	}
	fmt.Println("ROUTE                 WEIGHT  STATUS   FAILS  BLOCKED_UNTIL")
	for _, r := range rows {
		fmt.Printf("%-21s %6d  %-7s  %5d  %s\n", r.ID, r.Weight, r.Status, r.ConsecutiveFailures, router.FormatTime(r.BlockedUntil))
	}
	fmt.Printf("strategy=%s sticky=%v affinity=%d\n", cfg.Strategy.Ordinary, cfg.Strategy.StickyProbability, affinityN)
	sha := sync.SHA256
	if sha == "" {
		sha = "-"
	} else {
		sha = shortSHA(sha)
	}
	checked := sync.CheckedOn
	if checked == "" {
		checked = "-"
	}
	fmt.Printf("pi_sync checked_on=%s sha256=%s\n", checked, sha)
	return 0
}

func cmdSimulate(args []string) int {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	count := fs.Int("count", 100, "")
	seed := fs.Int("seed", 1, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg := config.MustLoad()
	st := state.Default(cfg)
	rng := newRNG(*seed)
	counts := map[string]int{}
	for _, route := range cfg.Ordinary.Routes {
		counts[route.ID] = 0
	}
	providers := map[string]int{}
	viaCounts := map[string]int{"affinity": 0, "weighted_random": 0}
	for i := 0; i < *count; i++ {
		route := router.Select(cfg, &st, 0, "", nil, rng)
		if route == nil {
			fmt.Fprintln(os.Stderr, "no eligible routes")
			return 1
		}
		counts[route.ID]++
		providers[route.Provider]++
		via := route.Via
		if via == "" {
			via = "weighted_random"
		}
		viaCounts[via]++
	}
	printJSON(map[string]any{
		"count":     *count,
		"seed":      *seed,
		"strategy":  cfg.Strategy.Ordinary,
		"routes":    counts,
		"providers": providers,
		"via":       viaCounts,
	})
	return 0
}

func cmdReport(args []string, success bool) int {
	if len(args) == 0 {
		return fail(fmt.Errorf("route id required"))
	}
	routeID := args[0]
	errMsg := ""
	if !success {
		for i := 1; i < len(args); i++ {
			if args[i] == "--error" && i+1 < len(args) {
				errMsg = args[i+1]
				i++
			}
		}
	}
	cfg := config.MustLoad()
	lookup := config.RouteByID(cfg)
	if _, ok := lookup[routeID]; !ok {
		return fail(fmt.Errorf("unknown route: %s", routeID))
	}
	var result map[string]any
	_ = state.WithLock(cfg, true, func(st *state.State) error {
		if success {
			router.MarkSuccess(cfg, st, routeID, -1)
			result = map[string]any{"ok": true, "route": routeID, "status": "whitelisted"}
			return nil
		}
		delay := router.MarkFailure(cfg, st, routeID, errMsg, -1)
		circuitID := lookup[routeID].Circuit
		result = map[string]any{
			"ok":            true,
			"route":         routeID,
			"circuit":       circuitID,
			"status":        "temporarily_blocked",
			"delay_seconds": delay,
			"blocked_until": st.Circuits[circuitID].BlockedUntil,
		}
		return nil
	})
	raw, _ := json.Marshal(result)
	fmt.Println(string(raw))
	return 0
}

func cmdReset(args []string) int {
	yes := false
	for _, a := range args {
		if a == "--yes" {
			yes = true
		}
	}
	if !yes {
		return fail(fmt.Errorf("reset requires --yes"))
	}
	cfg := config.MustLoad()
	_ = state.WithLock(cfg, true, func(st *state.State) error {
		sync := st.PiSync
		fresh := state.Default(cfg)
		*st = fresh
		st.PiSync = sync
		return nil
	})
	fmt.Println("qianji router state reset")
	return 0
}

func newRNG(seed int) *rand.Rand {
	return rand.New(rand.NewSource(int64(seed)))
}
