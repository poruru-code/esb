// Where: cli/internal/usecase/deploy/deploy_build_phase.go
// What: Build-phase preparation and invocation helpers for deploy workflow.
// Why: Isolate staging snapshot/build request logic from Run orchestration.
package deploy

func (w Workflow) runBuildPhase(req Request) error {
	return w.Build(w.buildRequest(req))
}
