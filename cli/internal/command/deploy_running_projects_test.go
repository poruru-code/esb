// Where: cli/internal/command/deploy_running_projects_test.go
// What: Unit tests for deploy running project discovery helpers.
// Why: Ensure project filtering and env inference from labels behave deterministically.
package command

import (
	"reflect"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
)

func TestInferEnvFromContainerLabels(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
		}
		got := inferEnvFromContainerLabels(containers)
		if got.Env != "dev" {
			t.Fatalf("expected env 'dev', got %q", got.Env)
		}
		if got.Source != "container label" {
			t.Fatalf("expected source 'container label', got %q", got.Source)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{compose.ESBEnvLabel: "dev"}},
			{Labels: map[string]string{compose.ESBEnvLabel: "prod"}},
		}
		got := inferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})

	t.Run("none", func(t *testing.T) {
		containers := []container.Summary{
			{Labels: map[string]string{}},
		}
		got := inferEnvFromContainerLabels(containers)
		if got.Env != "" {
			t.Fatalf("expected empty env, got %q", got.Env)
		}
	})
}

func TestExtractRunningDeployTargetStacks(t *testing.T) {
	containers := []container.Summary{
		{
			Names:  []string{"/esb-dev-gateway"},
			Labels: map[string]string{compose.ComposeServiceLabel: "gateway", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-dev-agent"},
			Labels: map[string]string{compose.ComposeServiceLabel: "agent", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-infra-registry"},
			Labels: map[string]string{compose.ComposeServiceLabel: "registry", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/buildx_buildkit_esb-buildx0"},
			Labels: map[string]string{},
		},
	}

	got := extractRunningDeployTargetStacks(containers)
	want := []deployTargetStack{
		{Name: "esb-dev", Project: "esb3", Env: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stacks: %#v", got)
	}
}

func TestExtractRunningDeployTargetStacksGatewayDown(t *testing.T) {
	containers := []container.Summary{
		{
			Names:  []string{"/esb-dev-agent"},
			Labels: map[string]string{compose.ComposeServiceLabel: "agent", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-dev-database"},
			Labels: map[string]string{compose.ComposeServiceLabel: "database", compose.ComposeProjectLabel: "esb3"},
		},
		{
			Names:  []string{"/esb-infra-registry"},
			Labels: map[string]string{compose.ComposeServiceLabel: "registry", compose.ComposeProjectLabel: "esb-infra"},
		},
		{
			Names:  []string{"/buildx_buildkit_esb-buildx0"},
			Labels: map[string]string{},
		},
	}

	got := extractRunningDeployTargetStacks(containers)
	want := []deployTargetStack{
		{Name: "esb-dev", Project: "esb3", Env: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stacks: %#v", got)
	}
}

func TestExtractRunningDeployTargetStacksPrefersGatewayMetadata(t *testing.T) {
	containers := []container.Summary{
		{
			Names:  []string{"/esb-dev-agent"},
			Labels: map[string]string{compose.ComposeServiceLabel: "agent", compose.ComposeProjectLabel: "project-from-agent"},
		},
		{
			Names:  []string{"/esb-dev-gateway"},
			Labels: map[string]string{compose.ComposeServiceLabel: "gateway", compose.ComposeProjectLabel: "project-from-gateway"},
		},
	}

	got := extractRunningDeployTargetStacks(containers)
	want := []deployTargetStack{
		{Name: "esb-dev", Project: "project-from-gateway", Env: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stacks: %#v", got)
	}
}

func TestResolveDeployTargetStackSingleAutoSelect(t *testing.T) {
	prompter := &recordingPrompter{}
	stack, err := resolveDeployTargetStack(
		[]deployTargetStack{{Name: "esb-dev", Project: "esb3", Env: "dev"}},
		true,
		prompter,
	)
	if err != nil {
		t.Fatalf("resolve target stack: %v", err)
	}
	if stack.Name != "esb-dev" {
		t.Fatalf("unexpected stack selected: %q", stack.Name)
	}
	if prompter.selectCalls != 0 {
		t.Fatalf("expected no prompt for single stack, got %d calls", prompter.selectCalls)
	}
}

func TestResolveDeployTargetStackMultipleNonTTY(t *testing.T) {
	_, err := resolveDeployTargetStack(
		[]deployTargetStack{
			{Name: "esb-dev", Project: "proj-a", Env: "dev"},
			{Name: "esb-prod", Project: "proj-b", Env: "prod"},
		},
		false,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error for multiple stacks without tty")
	}
	if !strings.Contains(err.Error(), errMultipleRunningProjects.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInferEnvFromStackName(t *testing.T) {
	if got := inferEnvFromStackName("esb-dev"); got != "dev" {
		t.Fatalf("expected dev env, got %q", got)
	}
	if got := inferEnvFromStackName("esb"); got != "" {
		t.Fatalf("expected empty env, got %q", got)
	}
}

func TestInferStackFromServiceName(t *testing.T) {
	if got := inferStackFromServiceName("esb-dev-runtime-node", "runtime-node"); got != "esb-dev" {
		t.Fatalf("expected esb-dev stack, got %q", got)
	}
	if got := inferStackFromServiceName("esb-dev-gateway", "agent"); got != "" {
		t.Fatalf("expected empty stack, got %q", got)
	}
	if got := inferStackFromServiceName("", "gateway"); got != "" {
		t.Fatalf("expected empty stack for blank name, got %q", got)
	}
}

type recordingPrompter struct {
	inputCalls  int
	inputValue  string
	selectCalls int
	selected    string
}

func (p *recordingPrompter) Input(_ string, _ []string) (string, error) {
	p.inputCalls++
	return p.inputValue, nil
}

func (p *recordingPrompter) Select(_ string, _ []string) (string, error) {
	p.selectCalls++
	if p.selected != "" {
		return p.selected, nil
	}
	return "", nil
}

func (p *recordingPrompter) SelectValue(_ string, _ []interaction.SelectOption) (string, error) {
	return "", nil
}
