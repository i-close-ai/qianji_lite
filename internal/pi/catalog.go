package pi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
)

const InstallHint = `qianji init 需要已安装官方 Pi agent，当前未找到 ` + "`pi`" + ` 命令。

安装 Pi（官方包，不要改源码）：
  sh -c "$(curl -fsSL https://raw.githubusercontent.com/i-close-ai/qianji_lite/main/tools/install.sh)"

或只装 Pi：
  npm install -g --ignore-scripts @earendil-works/pi-coding-agent

然后确认 ` + "`pi`" + ` 在 PATH 里（常见：~/.local/bin）。
自定义供应商和密钥请按官方 Pi 文档配置：
  https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/models.md
Qianji 通过 ` + "`pi --list-models`" + ` 询问 Pi 当前可用模型，不会替你安装或改写 Pi，也不会保存 API key。
`

func FindBinary(command string) string {
	if command == "" {
		command = "pi"
	}
	if filepath.IsAbs(command) {
		if isExec(command) {
			return command
		}
		return ""
	}
	if found, err := exec.LookPath(command); err == nil {
		return found
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	fallback := filepath.Join(home, ".local", "bin", command)
	if isExec(fallback) {
		return fallback
	}
	return ""
}

func RequireInstalled() (string, error) {
	if found := FindBinary("pi"); found != "" {
		return found, nil
	}
	return "", fmt.Errorf("%s", InstallHint)
}

func isExec(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func CatalogSHA256(catalog []config.CatalogItem) string {
	payload := make([][2]string, 0, len(catalog))
	for _, item := range catalog {
		payload = append(payload, [2]string{item.Provider, item.Model})
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func ParseListModels(text string) []config.CatalogItem {
	var catalog []config.CatalogItem
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lowered := strings.ToLower(line)
		if strings.HasPrefix(lowered, "provider") || strings.HasPrefix(lowered, "warning:") || strings.HasPrefix(lowered, "no models") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		provider, model := parts[0], parts[1]
		if strings.EqualFold(provider, "no") || strings.EqualFold(provider, "warning") {
			continue
		}
		key := provider + "\x00" + model
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		catalog = append(catalog, config.CatalogItem{Provider: provider, Model: model})
	}
	sortCatalog(catalog)
	return catalog
}

func FetchCatalog() ([]config.CatalogItem, string, error) {
	binary, err := RequireInstalled()
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--list-models")
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "FORCE_COLOR=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, "", fmt.Errorf("pi --list-models timed out after 30s")
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail == "" {
			detail = err.Error()
		}
		return nil, "", fmt.Errorf("pi --list-models failed: %s", detail)
	}
	catalog := ParseListModels(stdout.String())
	if len(catalog) == 0 {
		extra := ""
		if s := strings.TrimSpace(stderr.String()); s != "" {
			extra = "\n" + s
		}
		return nil, "", fmt.Errorf("Pi reported no available models. Configure custom providers in ~/.pi/agent/models.json, or authenticate official OpenAI/Anthropic with /login or env vars, then retry `qianji init`.%s", extra)
	}
	return catalog, CatalogSHA256(catalog), nil
}

func sortCatalog(catalog []config.CatalogItem) {
	for i := 0; i < len(catalog); i++ {
		for j := i + 1; j < len(catalog); j++ {
			if catalog[j].Provider < catalog[i].Provider || (catalog[j].Provider == catalog[i].Provider && catalog[j].Model < catalog[i].Model) {
				catalog[i], catalog[j] = catalog[j], catalog[i]
			}
		}
	}
}
