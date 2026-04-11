// internal/repl/repl.go
package repl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/89jobrien/devkit/internal/chain"
	"github.com/chzyer/readline"
)

// ErrExit is returned by DispatchCommand when the user requests exit/quit.
// The Run loop treats this as a clean shutdown, not an error.
var ErrExit = errors.New("repl: exit requested")

// DispatchConfig holds the runtime config for dispatching REPL commands.
// StageRunners and SynthesisRunner are injected by the cmd layer.
type DispatchConfig struct {
	StageRunners    map[string]chain.StageRunner
	SynthesisRunner chain.StageRunner
	RepoPath        string
}

// ParseCommand splits a raw input line into command, args, and --no-context flag.
// Returns empty command string for blank input.
func ParseCommand(line string) (cmd string, args []string, noContext bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil, false
	}
	var filtered []string
	for _, f := range fields[1:] {
		if f == "--no-context" {
			noContext = true
		} else {
			filtered = append(filtered, f)
		}
	}
	return fields[0], filtered, noContext
}

// DispatchCommand executes a single REPL command and returns its output.
// Built-in REPL commands: exit, clear, context, help.
// All other commands are delegated to the stage registry or chain pipeline.
func DispatchCommand(cmd string, args []string, noContext bool, s *Session, cfg DispatchConfig) (string, error) {
	switch cmd {
	case "exit", "quit":
		return "", ErrExit
	case "clear":
		s.Clear()
		return "session cleared.", nil
	case "context":
		summary := s.ContextSummary()
		if summary == "" {
			return "(no session context)", nil
		}
		return summary, nil
	case "help":
		return helpText(), nil
	case "chain":
		return dispatchChain(context.Background(), args, noContext, s, cfg)
	default:
		// Single-stage shorthand: "council" == "chain council"
		if cfg.StageRunners != nil {
			if _, ok := cfg.StageRunners[cmd]; ok {
				return dispatchChain(context.Background(), []string{cmd}, noContext, s, cfg)
			}
		}
		return "", fmt.Errorf("unknown command %q — type 'help' for available commands", cmd)
	}
}

func dispatchChain(ctx context.Context, stages []string, noContext bool, s *Session, cfg DispatchConfig) (string, error) {
	slots, err := chain.SelectStages(stages)
	if err != nil {
		return "", err
	}
	for i, slot := range slots {
		if slot.Selected && cfg.StageRunners != nil {
			if r, ok := cfg.StageRunners[slot.Name]; ok {
				slots[i].Runner = r
			}
		}
	}
	synthesis := cfg.SynthesisRunner
	if synthesis == nil {
		synthesis = chain.StageRunnerFunc(func(_ context.Context, _ []chain.Result) chain.Result {
			return chain.Result{Stage: "synthesis", Output: "(no synthesis runner configured)"}
		})
	}
	results, err := chain.RunPipeline(ctx, slots, synthesis)
	if err != nil {
		return "", err
	}
	// Accumulate all non-skipped results into session.
	for _, r := range results {
		s.AppendIfContext(r, !noContext)
	}
	// Return the synthesis output as the primary response.
	last := results[len(results)-1]
	if last.Err != nil {
		return "", last.Err
	}
	return last.Output, nil
}

func helpText() string {
	return `devkit repl — available commands:

  chain <stage>...    run selected stages in canonical order + synthesis
  council             shorthand for: chain council
  ci-triage           shorthand for: chain ci-triage
  log-pattern         shorthand for: chain log-pattern
  diagnose            shorthand for: chain diagnose
  ticket              shorthand for: chain ticket
  pr                  shorthand for: chain pr
  meta                shorthand for: chain meta
  context             show accumulated session context
  clear               clear session context
  help                show this message
  exit / quit         exit the REPL

Flags (per command):
  --no-context        run command without reading or writing session context
`
}

// Run starts the readline REPL loop. It blocks until the user exits.
// auth is pre-validated by the cmd layer before Run is called.
func Run(s *Session, cfg DispatchConfig) error {
	rl, err := readline.New("devkit> ")
	if err != nil {
		return fmt.Errorf("repl: readline init: %w", err)
	}
	defer rl.Close()

	// Handle Ctrl-C gracefully: close readline so the read loop exits cleanly.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		rl.Close()
	}()

	fmt.Println("devkit repl — type 'help' for commands, 'exit' to quit")
	for {
		line, err := rl.Readline()
		if err != nil { // EOF or Ctrl-D
			break
		}
		line = strings.TrimSpace(line)
		cmd, args, noCtx := ParseCommand(line)
		if cmd == "" {
			continue
		}
		out, dispErr := DispatchCommand(cmd, args, noCtx, s, cfg)
		if errors.Is(dispErr, ErrExit) {
			break
		}
		if dispErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", dispErr)
			continue
		}
		if out != "" {
			fmt.Println(out)
		}
	}
	return nil
}
