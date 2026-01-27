package runtime

import "github.com/poruru/edge-serverless-box/meta"

const (
	// LabelFunctionName is the label key for the function name.
	LabelFunctionName = meta.RuntimeLabelFunction

	// LabelCreatedBy is the label key for the creator identifier.
	LabelCreatedBy = meta.RuntimeLabelCreatedBy

	// ValueCreatedByAgent is the value for LabelCreatedBy.
	ValueCreatedByAgent = meta.RuntimeLabelCreatedByValue

	// LabelEsbEnv is the label key for the environment identifier.
	LabelEsbEnv = meta.RuntimeLabelEnv

	// LabelFunctionKind is the label key for function containers/images.
	LabelFunctionKind = meta.LabelPrefix + ".kind"

	// LabelOwner is the label key for the gateway owner identifier.
	LabelOwner = meta.LabelPrefix + ".owner"

	// ValueFunctionKind is the label value for function containers/images.
	ValueFunctionKind = "function"
)
