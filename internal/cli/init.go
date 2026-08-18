package cli

import (
	"fmt"
	"os"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/pi"
	"github.com/i-close-ai/qianji_lite/internal/state"
)

func cmdInit(args []string, forceReinit bool) int {
	force := forceReinit
	asJSON := false
	for _, a := range args {
		switch a {
		case "--force":
			force = true
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Println("qianji init [--force] [--json]")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", a)
			return 2
		}
	}
	if _, err := pi.RequireInstalled(); err != nil {
		return fail(err)
	}
	catalog, digest, err := pi.FetchCatalog()
	if err != nil {
		return fail(err)
	}
	if !force && fileExists(config.Path()) {
		cfg, err := config.Load()
		if err != nil {
			return fail(err)
		}
		var previous string
		_ = state.WithLock(cfg, false, func(st *state.State) error {
			previous = st.PiSync.SHA256
			return nil
		})
		if previous == digest {
			ids := make([]string, 0, len(cfg.Ordinary.Routes))
			for _, route := range cfg.Ordinary.Routes {
				ids = append(ids, route.ID)
			}
			result := syncResult{
				OK:           true,
				Reason:       "unchanged",
				SHA256:       digest,
				Config:       config.Path(),
				Source:       "pi --list-models",
				Models:       len(catalog),
				Added:        []string{},
				Kept:         ids,
				Removed:      []string{},
				DroppedTiers: []string{},
				Routes:       len(cfg.Ordinary.Routes),
			}
			if asJSON {
				printJSON(result)
			} else {
				fmt.Printf("qianji init: Pi catalog unchanged (%d models, %s…)\n", len(catalog), shortSHA(digest))
			}
			return 0
		}
	}
	reason := "init"
	if forceReinit {
		reason = "reinit"
	} else if force {
		reason = "init-force"
	}
	result, err := syncFromPi(force, reason, catalog, digest)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		printJSON(result)
		return 0
	}
	fmt.Printf("qianji init: wrote %s (kept %d, added %d, removed %d)\n",
		result.Config, len(result.Kept), len(result.Added), len(result.Removed))
	if len(result.Added) > 0 {
		fmt.Println("added (weight=1): " + join(result.Added))
	}
	if len(result.Removed) > 0 {
		fmt.Println("removed: " + join(result.Removed))
	}
	return 0
}

func join(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ", "
		}
		out += item
	}
	return out
}
