package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/pi"
	"github.com/i-close-ai/qianji_lite/internal/state"
)

const AppVersion = "2.0.0"

func Main(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}
	cmd := args[0]
	rest := args[1:]
	if cmd != "init" && cmd != "reinit" && cmd != "version" && cmd != "help" && cmd != "-h" && cmd != "--help" && cmd != "skill" && cmd != "doctor" && cmd != "--version" && cmd != "-v" {
		maybeDailySync()
	}
	switch cmd {
	case "version", "--version", "-v":
		fmt.Println(AppVersion)
		return 0
	case "help", "-h", "--help":
		printUsage()
		return 0
	case "init":
		return cmdInit(rest, false)
	case "reinit":
		return cmdInit(rest, true)
	case "status":
		return cmdStatus(rest)
	case "choose":
		return cmdChoose(rest)
	case "run":
		return cmdRun(rest)
	case "simulate":
		return cmdSimulate(rest)
	case "success":
		return cmdReport(rest, true)
	case "failure":
		return cmdReport(rest, false)
	case "reset":
		return cmdReset(rest)
	case "skill":
		return cmdSkill(rest)
	case "doctor":
		return cmdDoctor(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Qianji model router (Pi backend) %s

Usage:
  qianji init [--force] [--json]
             live-probes each listed model; only successes are imported
  qianji reinit [--json]
  qianji run [--prompt TEXT] [--prompt-file PATH] [--workdir DIR]
             [--timeout SEC] [--max-attempts N]
             [--tier TIER] [--route ID] [--provider NAME] [--model ID]
             [--effort LEVEL] [--affinity-key TEXT]
      --timeout 0 = auto: 900s ordinary / 1800s strong / 2400s strongest
  qianji status [--json]
  qianji choose [--json] [--affinity-key TEXT]
  qianji simulate [--count N] [--seed N]
  qianji skill install [--force]
  qianji doctor
  qianji version
`, AppVersion)
}

func catalogCheckedToday(checkedOn, previousSHA, today string) bool {
	return checkedOn == today && previousSHA != ""
}

func todayLocal() string {
	return time.Now().Format("2006-01-02")
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, strings.TrimRight(err.Error(), "\n"))
	return 1
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func maybeDailySync() {
	if pi.FindBinary("pi") == "" {
		fmt.Fprintln(os.Stderr, "qianji: Pi CLI not found; skip daily sync. Run `qianji init` after installing Pi.")
		return
	}
	today := todayLocal()
	cfg := config.MustLoad()
	var previous, checkedOn string
	_ = state.WithLock(cfg, false, func(st *state.State) error {
		previous = st.PiSync.SHA256
		checkedOn = st.PiSync.CheckedOn
		return nil
	})
	if catalogCheckedToday(checkedOn, previous, today) {
		return
	}
	catalog, digest, err := pi.FetchCatalog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "qianji: daily Pi catalog check failed: %v\n", err)
		return
	}
	if previous == digest && fileExists(config.Path()) {
		_ = state.WithLock(cfg, true, func(st *state.State) error {
			st.PiSync.CheckedOn = today
			st.PiSync.SHA256 = digest
			st.PiSync.Source = pi.CatalogSource
			st.PiSync.Models = len(cfg.Ordinary.Routes)
			return nil
		})
		return
	}
	reason := "daily-init"
	if previous != "" {
		reason = "daily-pi-config-change"
	}
	dropMissing := !fileExists(config.Path())
	result, err := syncFromPi(syncOpts{
		force:       false,
		reason:      reason,
		catalog:     catalog,
		digest:      digest,
		dropMissing: dropMissing,
		probeAll:    dropMissing,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "qianji: daily sync failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "qianji: synced from Pi (%s) sha256=%s… kept=%d added=%d stale=%d\n",
		result.Reason, shortSHA(result.SHA256), len(result.Kept), len(result.Added), len(result.Stale))
	if len(result.Added) > 0 {
		fmt.Fprintf(os.Stderr, "qianji: new routes at weight=1: %s\n", strings.Join(result.Added, ", "))
	}
	if len(result.ProbeFailed) > 0 {
		fmt.Fprintf(os.Stderr, "qianji: live probe excluded %s\n", strings.Join(result.ProbeFailed, ", "))
	}
	if len(result.Stale) > 0 {
		fmt.Fprintf(os.Stderr, "qianji: Pi catalog no longer lists %s (kept; run `qianji init` to drop)\n",
			strings.Join(result.Stale, ", "))
	}
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func nonempty(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

type syncResult struct {
	OK           bool              `json:"ok"`
	Reason       string            `json:"reason"`
	SHA256       string            `json:"sha256"`
	Config       string            `json:"config"`
	Source       string            `json:"source"`
	Models       int               `json:"models"`
	Catalog      int               `json:"catalog"`
	Probed       int               `json:"probed"`
	Added        []string          `json:"added"`
	Kept         []string          `json:"kept"`
	Removed      []string          `json:"removed"`
	DroppedTiers []string          `json:"dropped_tiers"`
	Stale        []string          `json:"stale"`
	StaleTiers   []string          `json:"stale_tiers"`
	ProbeOK      []string          `json:"probe_ok"`
	ProbeFailed  []string          `json:"probe_failed"`
	ProbeErrors  []pi.ProbeFailure `json:"probe_errors,omitempty"`
	Routes       int               `json:"routes"`
	Force        bool              `json:"force,omitempty"`
}

type syncOpts struct {
	force       bool
	reason      string
	catalog     []config.CatalogItem
	digest      string
	dropMissing bool
	probeAll    bool
}

var catalogFilter = pi.FilterCatalog

func syncFromPi(opts syncOpts) (syncResult, error) {
	catalog, digest := opts.catalog, opts.digest
	if catalog == nil {
		var err error
		catalog, digest, err = pi.FetchCatalog()
		if err != nil {
			return syncResult{}, err
		}
	}
	existing := config.Default()
	configPresent := fileExists(config.Path())
	if configPresent {
		var err error
		existing, err = config.Load()
		if err != nil {
			return syncResult{}, err
		}
	}
	cachedFailed := map[string]struct{}{}
	if !opts.probeAll && configPresent {
		_ = state.WithLock(existing, false, func(st *state.State) error {
			cachedFailed = probeFailedKeys(st.PiSync.ProbeFailed)
			return nil
		})
	}
	if opts.probeAll {
		fmt.Fprintf(os.Stderr, "qianji: live-probing %d models from pi --list-models (thinking=%s, no tools, timeout=%ds, concurrency=%d)\n",
			len(catalog), pi.ProbeThinking, pi.ProbeTimeoutSec, pi.ProbeConcurrency)
	}
	var progressMu sync.Mutex
	report, err := catalogFilter(catalog, pi.FilterOpts{
		ProbeAll:     opts.probeAll,
		Existing:     routeKeys(existing),
		CachedFailed: cachedFailed,
		Config:       existing,
		OnResult: func(out pi.ProbeOutcome) {
			progressMu.Lock()
			defer progressMu.Unlock()
			ref := pi.ItemRef(out.Item.Provider, out.Item.Model)
			status := "ok"
			if !out.OK {
				status = "fail"
			}
			if out.Error != "" {
				fmt.Fprintf(os.Stderr, "qianji probe: %-4s %s  %dms  %s\n", status, ref, out.ElapsedMs, out.Error)
				return
			}
			fmt.Fprintf(os.Stderr, "qianji probe: %-4s %s  %dms\n", status, ref, out.ElapsedMs)
		},
	})
	if err != nil {
		return syncResult{}, err
	}
	if opts.probeAll && len(report.Live) == 0 {
		return syncResult{}, fmt.Errorf("all %d listed models failed the live probe; existing config left unchanged", len(catalog))
	}
	generated := config.GenerateFromCatalog(report.Live, 10)
	var summary config.MergeSummary
	var merged config.Config
	if opts.dropMissing {
		merged, summary = config.Merge(existing, generated)
	} else {
		merged, summary = config.MergeKeepMissing(existing, generated)
	}
	changed := !configPresent || opts.probeAll || len(summary.Added) > 0 || len(summary.Removed) > 0 || len(summary.DroppedTiers) > 0
	if changed {
		if err := config.WriteAtomic(merged, digest); err != nil {
			return syncResult{}, err
		}
	}
	stateCfg := merged
	if !changed {
		stateCfg = existing
	}
	failedRefs := pi.FailedRefs(report, catalog, cachedFailed, opts.probeAll)
	_ = state.WithLock(stateCfg, true, func(st *state.State) error {
		st.PiSync = state.PiSync{
			SHA256:      digest,
			CheckedOn:   todayLocal(),
			Reason:      opts.reason,
			Force:       opts.force,
			Source:      pi.CatalogSource,
			Models:      len(report.Live),
			ProbeFailed: failedRefs,
			Probed:      report.Probed,
		}
		return nil
	})
	return syncResult{
		OK:           true,
		Reason:       opts.reason,
		SHA256:       digest,
		Config:       config.Path(),
		Source:       pi.CatalogSource,
		Models:       len(report.Live),
		Catalog:      len(catalog),
		Probed:       report.Probed,
		Added:        nonempty(summary.Added),
		Kept:         nonempty(summary.Kept),
		Removed:      nonempty(summary.Removed),
		DroppedTiers: nonempty(summary.DroppedTiers),
		Stale:        nonempty(summary.Stale),
		StaleTiers:   nonempty(summary.StaleTiers),
		ProbeOK:      nonempty(report.OK),
		ProbeFailed:  nonempty(failedRefs),
		ProbeErrors:  report.Failed,
		Routes:       summary.Routes,
		Force:        opts.force,
	}, nil
}

func routeKeys(cfg config.Config) map[string]struct{} {
	out := map[string]struct{}{}
	for _, route := range cfg.Ordinary.Routes {
		out[config.RouteKey(route.Provider, route.Model)] = struct{}{}
	}
	return out
}

func probeFailedKeys(refs []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, ref := range refs {
		provider, model := config.ParseModelRef(ref)
		if provider == "" || model == "" {
			continue
		}
		out[config.RouteKey(provider, model)] = struct{}{}
	}
	return out
}
