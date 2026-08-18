package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/i-close-ai/qianji_lite/internal/config"
	"github.com/i-close-ai/qianji_lite/skill"
)

func cmdSkill(args []string) int {
	if len(args) == 0 {
		fmt.Println("qianji skill install [--force]")
		fmt.Println("qianji skill path")
		return 0
	}
	switch args[0] {
	case "install":
		force := false
		for _, a := range args[1:] {
			if a == "--force" {
				force = true
			}
		}
		return skillInstall(force)
	case "path":
		fmt.Println(skillDest())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown skill command: %s\n", args[0])
		return 2
	}
}

func skillDest() string {
	return filepath.Join(config.Home(), "skill")
}

func skillInstall(force bool) int {
	dest := skillDest()
	if err := extractSkill(dest); err != nil {
		return fail(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fail(err)
	}
	links := []string{
		filepath.Join(home, ".agents", "skills", "qianji"),
		filepath.Join(home, ".cursor", "skills", "qianji"),
		filepath.Join(home, ".claude", "skills", "qianji"),
		filepath.Join(home, ".codex", "skills", "qianji"),
	}
	for _, link := range links {
		if err := linkSkill(link, dest, force); err != nil {
			fmt.Fprintf(os.Stderr, "qianji skill: skip %s: %v\n", link, err)
			continue
		}
		fmt.Printf("linked %s -> %s\n", link, dest)
	}
	fmt.Printf("skill installed at %s\n", dest)
	return 0
}

func extractSkill(dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(skill.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." || strings.HasSuffix(path, ".go") {
			return nil
		}
		target := filepath.Join(dest, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(skill.FS, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func linkSkill(link, dest string, force bool) error {
	parent := filepath.Dir(link)
	hostRoot := filepath.Dir(parent)
	if filepath.Base(hostRoot) != ".agents" && !dirExists(hostRoot) {
		return fmt.Errorf("host directory missing")
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	info, err := os.Lstat(link)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			current, _ := os.Readlink(link)
			if current == dest && !force {
				return nil
			}
			if err := os.Remove(link); err != nil {
				return err
			}
		} else if force {
			if err := os.RemoveAll(link); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("exists and is not a symlink (use --force)")
		}
	}
	return os.Symlink(dest, link)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
