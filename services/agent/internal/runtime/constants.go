package runtime

import "github.com/poruru-code/esb/services/agent/internal/identity"

var (
	// LabelFunctionName is the label key for the function name.
	LabelFunctionName string

	// LabelCreatedBy is the label key for the creator identifier.
	LabelCreatedBy string

	// ValueCreatedByAgent is the value for LabelCreatedBy.
	ValueCreatedByAgent string

	// LabelEsbEnv is the label key for the environment identifier.
	LabelEsbEnv string

	// LabelFunctionKind is the label key for function containers/images.
	LabelFunctionKind string

	// LabelOwner is the label key for the gateway owner identifier.
	LabelOwner string

	// ValueFunctionKind is the label value for function containers/images.
	ValueFunctionKind = "function"
)

const bootstrapBrandSlug = "esb"

func init() {
	ApplyIdentity(identity.StackIdentity{BrandSlug: bootstrapBrandSlug, Source: "bootstrap"})
}

func ApplyIdentity(id identity.StackIdentity) {
	LabelFunctionName = id.RuntimeLabelFunction()
	LabelCreatedBy = id.RuntimeLabelCreatedBy()
	ValueCreatedByAgent = id.RuntimeLabelCreatedByValue()
	LabelEsbEnv = id.RuntimeLabelEnv()
	LabelFunctionKind = id.LabelPrefix() + ".kind"
	LabelOwner = id.LabelPrefix() + ".owner"
}
