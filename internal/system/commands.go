package system

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// commandTimeout bounds any external command so a report cannot hang forever.
const commandTimeout = 3 * time.Second

// commandRunner makes external commands injectable in tests.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execCommandRunner is the production command runner.
type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// runCommand applies a short deadline to a command invocation. Applying the
// timeout here (rather than only in the production runner) also lets tests
// verify that injected runners receive a bounded context.
func runCommand(ctx context.Context, runner commandRunner, name string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if runner == nil {
		return nil, errors.New("system: no command runner configured")
	}

	commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	return runner.Run(commandCtx, name, args...)
}
