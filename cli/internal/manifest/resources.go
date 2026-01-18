package manifest

import "github.com/poruru-code/aws-sam-parser-go/schema"

// ResourcesSpec defines the desired state of resources to be provisioned.
// This is the "Intent" or "Manifest" that the Provisioner will use to apply changes.
//
// NOTE: This package depends on github.com/poruru-code/aws-sam-parser-go/schema.
// If we want to eliminate this dependency in the future, we should define our own
// versions of these schema types here.
type ResourcesSpec struct {
	DynamoDB []DynamoDBSpec
	S3       []S3Spec
	Layers   []LayerSpec
}

// DynamoDBSpec defines the parameters for a DynamoDB table.
type DynamoDBSpec struct {
	TableName              string
	KeySchema              []schema.AWSDynamoDBTableKeySchema
	AttributeDefinitions   []schema.AWSDynamoDBTableAttributeDefinition
	GlobalSecondaryIndexes []schema.AWSDynamoDBTableGlobalSecondaryIndex
	BillingMode            string
	ProvisionedThroughput  *schema.AWSDynamoDBTableProvisionedThroughput
}

// S3Spec defines the parameters for an S3 bucket.
type S3Spec struct {
	BucketName             string
	LifecycleConfiguration *schema.AWSS3BucketLifecycleConfiguration
}

// LayerSpec defines the parameters for a Lambda Layer.
type LayerSpec struct {
	Name                    string
	ContentURI              string
	CompatibleArchitectures []string
}

// BuildRequest contains parameters for a build operation.
type BuildRequest struct {
	ProjectDir   string
	TemplatePath string
	Env          string
	NoCache      bool
	Verbose      bool
}
