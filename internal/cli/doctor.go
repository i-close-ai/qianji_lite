package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/internal/pi"
)

func cmdDoctor(args []string) int {
	_ = args
	home, _ := os.UserHomeDir()
	ok := true
	check := func(name, detail string, pass bool) {
		mark := "ok"
		if !pass {
			mark = "FAIL"
			ok = false
		}
		fmt.Printf("%-12s %s  %s\n", mark, name, detail)
	}

	piBin := pi.FindBinary("pi")
	check("pi", orDefault(piBin, "not found — npm install -g --ignore-scripts @earendil-works/pi-coding-agent"), piBin != "")
	check("config", config.Path(), fileExists(config.Path()))
	check("skill", skillDest(), fileExists(filepath.Join(skillDest(), "SKILL.md")))

	if os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" {
		fmt.Printf("%-12s %s  %s\n", "warn", "anthropic-env", "set; pi --list-models lists Anthropic models even without Pi /login; qianji init live-probe will drop them if the call fails")
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		fmt.Printf("%-12s %s  %s\n", "warn", "openai-env", "set; pi --list-models lists OpenAI models even without Pi /login; qianji init live-probe will drop them if the call fails")
	}

	links := []struct{ name, path string }{
		{"agents", filepath.Join(home, ".agents", "skills", "qianji")},
		{"cursor", filepath.Join(home, ".cursor", "skills", "qianji")},
		{"claude", filepath.Join(home, ".claude", "skills", "qianji")},
		{"codex", filepath.Join(home, ".codex", "skills", "qianji")},
	}
	for _, item := range links {
		target, err := os.Readlink(item.path)
		if err != nil {
			if fileExists(item.path) {
				check(item.name, item.path+" (not a symlink)", true)
			} else if dirExists(filepath.Dir(filepath.Dir(item.path))) || item.name == "agents" {
				check(item.name, "not linked — run qianji skill install", false)
			} else {
				fmt.Printf("%-12s %s  host not installed\n", "skip", item.name)
			}
			continue
		}
		check(item.name, item.path+" -> "+target, true)
	}
	if !ok {
		return 1
	}
	return 0
}
