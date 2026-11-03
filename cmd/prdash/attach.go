package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/sovengar/agentbridge"

	"prdash/internal/config"
	"prdash/internal/herdr"
)

// attachArgv builds the argv for `prdash attach`. The shared ai capability
// table decides the agent flags, and an agent whose interactive UI takes no
// initial prompt falls back to its headless form so the prompt is not lost.
func attachArgv(cfg config.AIConfig, prompt string) ([]string, error) {
	if cfg.Harness == "" {
		return nil, fmt.Errorf("no AI harness configured (agentbridge.harness in the config)")
	}
	req := agentbridge.Request{
		Agent:  agentbridge.Agent{Name: cfg.Harness, Binary: cfg.Harness},
		Model:  cfg.Model,
		Effort: cfg.Effort,
		Prompt: prompt,
	}
	caps := agentbridge.CapabilitiesFor(cfg.Harness)
	if caps.NoInteractivePrompt && prompt != "" {
		return agentbridge.HeadlessArgs(req)
	}
	return agentbridge.InteractiveArgs(req), nil
}

// runAttach starts the configured agent with the prompt in a herdr pane when
// herdr is available, and in the foreground otherwise.
func runAttach(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	prompt := strings.TrimSpace(strings.Join(args, " "))
	argv, err := attachArgv(cfg.AI, prompt)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 2
	}
	if len(argv) == 0 || argv[0] == "" {
		_, _ = fmt.Fprintln(stderr, "prdash: empty ai argv")
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 1
	}

	client := herdr.New()
	if client.Available() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pane, err := client.PaneSplit(ctx, herdr.SplitSpec{Direction: "down", Cwd: cwd})
		if err == nil {
			if err := client.PaneRun(ctx, pane.PaneID, argv); err != nil {
				_, _ = fmt.Fprintln(stderr, "prdash: herdr pane run:", err)
				return 1
			}
			_, _ = fmt.Fprintf(stdout, "%s attached in herdr pane %s\n", cfg.AI.Harness, pane.PaneID)
			return 0
		}
		// A pane that cannot be created is not fatal: the agent still runs in
		// the foreground, which is what the user asked for.
		_, _ = fmt.Fprintln(stderr, "prdash: herdr pane split:", err)
	}

	path, err := exec.LookPath(argv[0])
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "prdash: %s not installed\n", argv[0])
		return 1
	}
	// The agent is interactive: it replaces this process so it keeps the TTY,
	// the signals and the exit code.
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 1
	}
	return 0
}
