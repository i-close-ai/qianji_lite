package pi

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
)

const (
	ProbePrompt      = "Reply with pong"
	ProbeTimeoutSec  = 60
	ProbeConcurrency = 4
	ProbeThinking    = "off"
	CatalogSource    = "pi --list-models + live probe"
)

var probeExtraArgs = []string{
	"--no-tools",
	"--no-skills",
	"--no-extensions",
	"--no-context-files",
	"--no-approve",
}

// ProbeFn runs one cheap live check. Tests inject a fake.
type ProbeFn func(item config.CatalogItem) ExecResult

type ProbeFailure struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	ID        string `json:"id"`
	Ref       string `json:"ref"`
	Error     string `json:"error"`
	ErrorType string `json:"error_type,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms"`
}

type ProbeOutcome struct {
	Item      config.CatalogItem
	OK        bool
	Skipped   bool
	Reason    string
	Error     string
	ErrorType string
	ElapsedMs int64
}

type ProbeReport struct {
	Live        []config.CatalogItem
	OK          []string
	Failed      []ProbeFailure
	SkippedFail []string
	Probed      int
}

type FilterOpts struct {
	ProbeAll     bool
	Existing     map[string]struct{}
	CachedFailed map[string]struct{}
	Config       config.Config
	Concurrency  int
	TimeoutSec   int
	Probe        ProbeFn
	OnResult     func(ProbeOutcome)
}

func ItemRef(provider, model string) string {
	return provider + "/" + model
}

func ProbeRoute(cfg config.Config, item config.CatalogItem, workdir string, timeoutSec int) ExecResult {
	if timeoutSec <= 0 {
		timeoutSec = ProbeTimeoutSec
	}
	route := config.Route{
		ID:       config.DefaultRouteID(item.Provider, item.Model),
		Circuit:  item.Provider + ":" + item.Model,
		Provider: item.Provider,
		Model:    item.Model,
		Effort:   ProbeThinking,
	}
	env := append(os.Environ(), "NO_COLOR=1", "FORCE_COLOR=0")
	return runRoute(cfg, route, ProbePrompt, workdir, timeoutSec, probeExtraArgs, env)
}

func FilterCatalog(catalog []config.CatalogItem, opts FilterOpts) (ProbeReport, error) {
	if opts.Existing == nil {
		opts.Existing = map[string]struct{}{}
	}
	if opts.CachedFailed == nil {
		opts.CachedFailed = map[string]struct{}{}
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = ProbeConcurrency
	}
	if opts.TimeoutSec <= 0 {
		opts.TimeoutSec = ProbeTimeoutSec
	}

	probe := opts.Probe
	if probe == nil {
		workdir, err := os.MkdirTemp("", "qianji-probe-*")
		if err != nil {
			return ProbeReport{}, fmt.Errorf("probe workdir: %w", err)
		}
		defer os.RemoveAll(workdir)
		cfg := opts.Config
		timeout := opts.TimeoutSec
		probe = func(item config.CatalogItem) ExecResult {
			return ProbeRoute(cfg, item, workdir, timeout)
		}
	}

	type job struct {
		index  int
		item   config.CatalogItem
		action string
	}
	jobs := make([]job, len(catalog))
	var toProbe []int
	for i, item := range catalog {
		key := config.RouteKey(item.Provider, item.Model)
		action := "probe"
		if !opts.ProbeAll {
			if _, ok := opts.Existing[key]; ok {
				action = "keep"
			} else if _, ok := opts.CachedFailed[key]; ok {
				action = "cached_failure"
			}
		}
		jobs[i] = job{index: i, item: item, action: action}
		if action == "probe" {
			toProbe = append(toProbe, i)
		}
	}

	outcomes := make([]ProbeOutcome, len(catalog))
	for i, j := range jobs {
		switch j.action {
		case "keep":
			outcomes[i] = ProbeOutcome{Item: j.item, OK: true, Skipped: true, Reason: "existing"}
		case "cached_failure":
			outcomes[i] = ProbeOutcome{Item: j.item, OK: false, Skipped: true, Reason: "cached_failure"}
		}
	}

	if len(toProbe) > 0 {
		workers := opts.Concurrency
		if workers > len(toProbe) {
			workers = len(toProbe)
		}
		indexes := make(chan int)
		var wg sync.WaitGroup
		wg.Add(workers)
		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := range indexes {
					item := catalog[i]
					started := time.Now()
					res := probe(item)
					elapsed := time.Since(started).Milliseconds()
					if elapsed < 0 {
						elapsed = 0
					}
					outcomes[i] = ProbeOutcome{
						Item:      item,
						OK:        res.OK,
						Reason:    "probed",
						Error:     res.Error,
						ErrorType: res.ErrorType,
						ElapsedMs: elapsed,
					}
					if opts.OnResult != nil {
						opts.OnResult(outcomes[i])
					}
				}
			}()
		}
		for _, i := range toProbe {
			indexes <- i
		}
		close(indexes)
		wg.Wait()
	}

	report := ProbeReport{}
	for _, out := range outcomes {
		ref := ItemRef(out.Item.Provider, out.Item.Model)
		id := config.DefaultRouteID(out.Item.Provider, out.Item.Model)
		if out.Reason == "probed" {
			report.Probed++
		}
		if out.OK {
			report.Live = append(report.Live, out.Item)
			report.OK = append(report.OK, id)
			continue
		}
		if out.Skipped && out.Reason == "cached_failure" {
			report.SkippedFail = append(report.SkippedFail, ref)
			continue
		}
		report.Failed = append(report.Failed, ProbeFailure{
			Provider:  out.Item.Provider,
			Model:     out.Item.Model,
			ID:        id,
			Ref:       ref,
			Error:     out.Error,
			ErrorType: out.ErrorType,
			ElapsedMs: out.ElapsedMs,
		})
	}
	return report, nil
}

func FailedRefs(report ProbeReport, catalog []config.CatalogItem, cachedFailed map[string]struct{}, probeAll bool) []string {
	failed := map[string]struct{}{}
	if !probeAll {
		inCatalog := map[string]struct{}{}
		for _, item := range catalog {
			inCatalog[config.RouteKey(item.Provider, item.Model)] = struct{}{}
		}
		for key := range cachedFailed {
			if _, ok := inCatalog[key]; ok {
				failed[key] = struct{}{}
			}
		}
	}
	for _, item := range report.Failed {
		failed[config.RouteKey(item.Provider, item.Model)] = struct{}{}
	}
	out := make([]string, 0, len(failed))
	for _, item := range catalog {
		key := config.RouteKey(item.Provider, item.Model)
		if _, ok := failed[key]; ok {
			out = append(out, ItemRef(item.Provider, item.Model))
		}
	}
	return out
}
