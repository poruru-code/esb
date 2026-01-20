// Where: cli/internal/workflows/workflow_test_helpers.go
// What: Test helpers and stub ports for workflow unit tests.
// Why: Keep workflow tests focused on orchestration behavior without external dependencies.
package workflows

import (
	"context"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

type testBlock struct {
	title string
	rows  []ports.KeyValue
}

type testUI struct {
	infos     []string
	warns     []string
	successes []string
	blocks    []testBlock
}

func (u *testUI) Info(msg string) {
	u.infos = append(u.infos, msg)
}

func (u *testUI) Warn(msg string) {
	u.warns = append(u.warns, msg)
}

func (u *testUI) Success(msg string) {
	u.successes = append(u.successes, msg)
}

func (u *testUI) Block(_, title string, rows []ports.KeyValue) {
	u.blocks = append(u.blocks, testBlock{title: title, rows: rows})
}

type recordEnvApplier struct {
	calls []state.Context
}

func (r *recordEnvApplier) Apply(ctx state.Context) {
	r.calls = append(r.calls, ctx)
}

type recordBuilder struct {
	requests []generator.BuildRequest
	err      error
}

func (r *recordBuilder) Build(req generator.BuildRequest) error {
	r.requests = append(r.requests, req)
	return r.err
}

type recordUpper struct {
	requests []ports.UpRequest
	err      error
}

func (r *recordUpper) Up(req ports.UpRequest) error {
	r.requests = append(r.requests, req)
	return r.err
}

type downCall struct {
	project string
	volumes bool
}

type recordDowner struct {
	calls []downCall
	err   error
}

func (r *recordDowner) Down(project string, removeVolumes bool) error {
	r.calls = append(r.calls, downCall{project: project, volumes: removeVolumes})
	return r.err
}

type recordPortPublisher struct {
	calls  []state.Context
	result ports.PortPublishResult
	err    error
}

func (r *recordPortPublisher) Publish(ctx state.Context) (ports.PortPublishResult, error) {
	r.calls = append(r.calls, ctx)
	if r.err != nil {
		return ports.PortPublishResult{}, r.err
	}
	return r.result, nil
}

type recordCredentialManager struct {
	creds  ports.AuthCredentials
	called int
}

func (r *recordCredentialManager) Ensure() ports.AuthCredentials {
	r.called++
	return r.creds
}

type recordTemplateLoader struct {
	paths   []string
	content string
	err     error
}

func (r *recordTemplateLoader) Read(path string) (string, error) {
	r.paths = append(r.paths, path)
	if r.err != nil {
		return "", r.err
	}
	return r.content, nil
}

type recordTemplateParser struct {
	contents []string
	result   generator.ParseResult
	err      error
}

func (r *recordTemplateParser) Parse(content string, _ map[string]string) (generator.ParseResult, error) {
	r.contents = append(r.contents, content)
	if r.err != nil {
		return generator.ParseResult{}, r.err
	}
	return r.result, nil
}

type provisionCall struct {
	resources manifest.ResourcesSpec
	project   string
}

type recordProvisioner struct {
	calls []provisionCall
	err   error
}

func (r *recordProvisioner) Apply(_ context.Context, resources manifest.ResourcesSpec, project string) error {
	r.calls = append(r.calls, provisionCall{resources: resources, project: project})
	return r.err
}

type recordWaiter struct {
	calls []state.Context
	err   error
}

func (r *recordWaiter) Wait(ctx state.Context) error {
	r.calls = append(r.calls, ctx)
	return r.err
}

type recordLogger struct {
	requests   []ports.LogsRequest
	listed     []string
	containers []state.ContainerInfo
	err        error
	listErr    error
}

func (r *recordLogger) Logs(req ports.LogsRequest) error {
	r.requests = append(r.requests, req)
	return r.err
}

func (r *recordLogger) ListServices(_ ports.LogsRequest) ([]string, error) {
	return r.listed, r.listErr
}

func (r *recordLogger) ListContainers(_ string) ([]state.ContainerInfo, error) {
	return r.containers, r.listErr
}

type recordStopper struct {
	requests []ports.StopRequest
	err      error
}

func (r *recordStopper) Stop(req ports.StopRequest) error {
	r.requests = append(r.requests, req)
	return r.err
}

type recordPruner struct {
	requests []ports.PruneRequest
	err      error
}

func (r *recordPruner) Prune(req ports.PruneRequest) error {
	r.requests = append(r.requests, req)
	return r.err
}
