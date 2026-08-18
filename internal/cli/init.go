package cli

import (
	"fmt"
	"os"

	"github.com/i-close-ai/qianji_lite/internal/pi"
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
	reason := "init"
	if forceReinit {
		reason = "reinit"
	} else if force {
		reason = "init-force"
	}
	result, err := syncFromPi(syncOpts{
		force:       force,
		reason:      reason,
		catalog:     catalog,
		digest:      digest,
		dropMissing: true,
		probeAll:    true,
	})
	if err != nil {
		return fail(err)
	}
	if asJSON {
		printJSON(result)
		return 0
	}
	fmt.Printf("qianji init: wrote %s (kept %d, added %d, removed %d, live %d/%d)\n",
		result.Config, len(result.Kept), len(result.Added), len(result.Removed), result.Models, result.Catalog)
	if len(result.Added) > 0 {
		fmt.Println("added (weight=1): " + join(result.Added))
	}
	if len(result.Removed) > 0 {
		fmt.Println("removed: " + join(result.Removed))
	}
	if len(result.ProbeFailed) > 0 {
		fmt.Println("excluded (live probe failed): " + join(result.ProbeFailed))
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
