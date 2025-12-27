package runtime

const (
	// ContainerNamePrefix is the prefix for all containers managed by the agent
	ContainerNamePrefix = "lambda-"

	// LabelFunctionName is the label key for the function name
	LabelFunctionName = "esb_function"

	// LabelCreatedBy is the label key for the creator identifier
	LabelCreatedBy = "created_by"

	// ValueCreatedByAgent is the value for LabelCreatedBy
	ValueCreatedByAgent = "esb-agent"
)
