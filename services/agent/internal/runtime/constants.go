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
)
