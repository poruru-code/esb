package workflows

import (
	"context"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type recordBuilder struct {
	requests []generator.BuildRequest
	err      error
}

func (b *recordBuilder) Build(request generator.BuildRequest) error {
	b.requests = append(b.requests, request)
	return b.err
}

type recordEnvApplier struct {
	applied []state.Context
}

func (a *recordEnvApplier) Apply(ctx state.Context) error {
	a.applied = append(a.applied, ctx)
	return nil
}

type testUI struct {
	success []string
	info    []string
	warn    []string
	error   []string
}

func (u *testUI) Success(msg string) {
	u.success = append(u.success, msg)
}

func (u *testUI) Info(msg string) {
	u.info = append(u.info, msg)
}

func (u *testUI) Warn(msg string) {
	u.warn = append(u.warn, msg)
}

func (u *testUI) Error(msg string) {
	u.error = append(u.error, msg)
}

func (u *testUI) Block(_, _ string, _ []ports.KeyValue) {
	// Simple mock implementation
}

// fakeComposeRunner is a test double for compose.CommandRunner.
type fakeComposeRunner struct {
	commands [][]string
	err      error
}

func (r *fakeComposeRunner) Run(ctx context.Context, dir, name string, args ...string) error {
	_ = ctx
	_ = dir
	r.commands = append(r.commands, append([]string{name}, args...))
	return r.err
}

func (r *fakeComposeRunner) RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	_ = ctx
	_ = dir
	r.commands = append(r.commands, append([]string{name}, args...))
	return nil, r.err
}

func (r *fakeComposeRunner) RunQuiet(ctx context.Context, dir, name string, args ...string) error {
	_ = ctx
	_ = dir
	r.commands = append(r.commands, append([]string{name}, args...))
	return r.err
}

// fakeRegistryChecker is a test double for RegistryChecker.
type fakeRegistryChecker struct{}

func (c *fakeRegistryChecker) WaitReady(registry string, timeout time.Duration) error {
	_ = registry
	_ = timeout
	return nil
}
