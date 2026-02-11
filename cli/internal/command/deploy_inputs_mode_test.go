// Where: cli/internal/command/deploy_inputs_mode_test.go
// What: Tests for runtime mode prompt behavior.
// Why: Ensure validation warnings are routed to the injected error writer.
package command

import (
	"bytes"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
)

type modeSelectPrompter struct {
	values []string
	index  int
}

func (p *modeSelectPrompter) Input(_ string, _ []string) (string, error) {
	return "", nil
}

func (p *modeSelectPrompter) Select(_ string, _ []string) (string, error) {
	if p.index >= len(p.values) {
		return "", nil
	}
	value := p.values[p.index]
	p.index++
	return value, nil
}

func (p *modeSelectPrompter) SelectValue(_ string, _ []interaction.SelectOption) (string, error) {
	return "", nil
}

func TestResolveDeployModeWritesWarningToConfiguredErrWriter(t *testing.T) {
	prompter := &modeSelectPrompter{values: []string{"", "containerd"}}
	var errOut bytes.Buffer

	got, err := resolveDeployMode("", true, prompter, "docker", &errOut)
	if err != nil {
		t.Fatalf("resolve deploy mode: %v", err)
	}
	if got != "containerd" {
		t.Fatalf("expected containerd, got %q", got)
	}
	if errOut.String() == "" {
		t.Fatalf("expected warning output when first selection is empty")
	}
}
