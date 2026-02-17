package runtime

import (
	"testing"

	"github.com/poruru/edge-serverless-box/services/agent/internal/identity"
)

func TestApplyIdentityUpdatesRuntimeLabels(t *testing.T) {
	origFunction := LabelFunctionName
	origCreatedBy := LabelCreatedBy
	origCreatedByValue := ValueCreatedByAgent
	origEnv := LabelEsbEnv
	origKind := LabelFunctionKind
	origOwner := LabelOwner
	t.Cleanup(func() {
		LabelFunctionName = origFunction
		LabelCreatedBy = origCreatedBy
		ValueCreatedByAgent = origCreatedByValue
		LabelEsbEnv = origEnv
		LabelFunctionKind = origKind
		LabelOwner = origOwner
	})

	ApplyIdentity(identity.StackIdentity{BrandSlug: "acme"})

	if LabelFunctionName != "acme_function" {
		t.Fatalf("LabelFunctionName = %q", LabelFunctionName)
	}
	if LabelCreatedBy != "created_by" {
		t.Fatalf("LabelCreatedBy = %q", LabelCreatedBy)
	}
	if ValueCreatedByAgent != "acme-agent" {
		t.Fatalf("ValueCreatedByAgent = %q", ValueCreatedByAgent)
	}
	if LabelEsbEnv != "acme_env" {
		t.Fatalf("LabelEsbEnv = %q", LabelEsbEnv)
	}
	if LabelFunctionKind != "com.acme.kind" {
		t.Fatalf("LabelFunctionKind = %q", LabelFunctionKind)
	}
	if LabelOwner != "com.acme.owner" {
		t.Fatalf("LabelOwner = %q", LabelOwner)
	}
}
