// Copyright 2025 Deductive AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/logging"
)

const (
	// HookTimeout is the maximum time a hook can run
	HookTimeout = 30 * time.Second
)

// RunHooks executes all hook scripts and returns combined output
func RunHooks(hooks []string) (string, error) {
	if len(hooks) == 0 {
		return "", nil
	}

	var outputs []string
	var errors []string

	for _, hookPath := range hooks {
		start := time.Now()
		output, err := runSingleHook(hookPath)
		logging.Debug("Hook executed", "path", hookPath, "duration", time.Since(start), "error", err)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Hook %s failed: %v", hookPath, err))
			continue
		}
		if output != "" {
			outputs = append(outputs, output)
		}
	}

	combined := strings.Join(outputs, "\n---\n")

	if len(errors) > 0 {
		return combined, fmt.Errorf("%s", strings.Join(errors, "; "))
	}

	return combined, nil
}

// runSingleHook executes a single hook script with timeout
func runSingleHook(hookPath string) (string, error) {
	cmd := exec.Command("bash", hookPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start: %w", err)
	}

	// Wait with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			// Include stderr in error message
			errMsg := err.Error()
			if stderr.Len() > 0 {
				errMsg = fmt.Sprintf("%s: %s", errMsg, stderr.String())
			}
			return "", fmt.Errorf("%s", errMsg)
		}
		return strings.TrimSpace(stdout.String()), nil
	case <-time.After(HookTimeout):
		cmd.Process.Kill()
		return "", fmt.Errorf("timed out after %v", HookTimeout)
	}
}

// FormatAppendix wraps output in <appendix> tags
func FormatAppendix(output string) string {
	if output == "" {
		return ""
	}
	return fmt.Sprintf("<appendix>\n%s\n</appendix>", output)
}

// ShouldIncludeAppendix returns true if the hook output should be included
// (i.e., it's different from the previous output)
func ShouldIncludeAppendix(current, previous string) bool {
	// Always include if there's output and it's different from previous
	if current == "" {
		return false
	}
	return current != previous
}
