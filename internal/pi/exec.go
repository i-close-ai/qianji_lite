package pi

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/i-close-ai/qianji_lite/internal/config"
)

type ExecResult struct {
	OK        bool
	Output    string
	Error     string
	ErrorType string
}

func LooksLikeFailure(returncode int, output string) bool {
	text := strings.TrimSpace(output)
	if returncode != 0 || text == "" {
		return true
	}
	head := failureHead(text)
	prefixes := []string{
		"API call failed after ",
		"No inference provider configured",
		"Authentication failed",
		"Error: HTTP ",
		"HTTP 4",
		"HTTP 5",
		"No API key found",
		"Billing or credits exhausted",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(head, prefix) {
			return true
		}
	}
	lowered := strings.ToLower(head)
	markers := []string{
		"billing, credits, or account entitlement is exhausted",
		"exceeded your current quota",
		"invalid api key",
		"no api key found",
		"authentication failed",
	}
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func failureHead(text string) string {
	var b strings.Builder
	n := 0
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if n > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		n++
		if n >= 3 || b.Len() >= 2000 {
			break
		}
	}
	return b.String()
}

func FindConfigured(cfg config.Config) (string, error) {
	command := cfg.Executor.Command
	if command == "" {
		command = "pi"
	}
	if found := FindBinary(command); found != "" {
		return found, nil
	}
	return "", fmtInstall()
}

func fmtInstall() error {
	return fmtError(InstallHint)
}

type hintError struct{ s string }

func (e hintError) Error() string { return e.s }

func fmtError(s string) error { return hintError{s} }

func RunRoute(cfg config.Config, route config.Route, prompt, workdir string, timeoutSec int) ExecResult {
	binary, err := FindConfigured(cfg)
	if err != nil {
		return ExecResult{OK: false, Error: err.Error(), ErrorType: "provider_failure"}
	}
	args := []string{
		"--provider", route.Provider,
		"--model", route.Model,
		"--print",
		"--no-session",
	}
	if route.Effort != "" {
		args = append(args, "--thinking", route.Effort)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = strings.NewReader(prompt)
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	output := string(out)
	if ctx.Err() == context.DeadlineExceeded {
		return ExecResult{OK: false, Output: output, Error: "timeout after " + strconv.Itoa(timeoutSec) + "s", ErrorType: "timeout"}
	}
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	if LooksLikeFailure(code, output) {
		summary := firstLine(output)
		if summary == "" {
			summary = "exit " + strconv.Itoa(code)
		}
		return ExecResult{OK: false, Output: output, Error: summary, ErrorType: "provider_failure"}
	}
	return ExecResult{OK: true, Output: output}
}

func firstLine(output string) string {
	text := strings.TrimSpace(output)
	if text == "" {
		return ""
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[:i])
	}
	return text
}
