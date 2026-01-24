// Where: cli/internal/manifest/resources.go
// What: Shared resource definitions for Generator and Provisioner.
// Why: Decouple parsing logic from provisioning logic.
package manifest

import "github.com/poruru-code/aws-sam-parser-go/schema"

type LayerSpec struct {
	Name                    string
	ContentURI              string
	CompatibleArchitectures []string
}

type DynamoDBSpec struct {
	TableName              string
	KeySchema              []schema.AWSDynamoDBTableKeySchema
	AttributeDefinitions   []schema.AWSDynamoDBTableAttributeDefinition
	GlobalSecondaryIndexes []schema.AWSDynamoDBTableGlobalSecondaryIndex
	BillingMode            string
	ProvisionedThroughput  *schema.AWSDynamoDBTableProvisionedThroughput
}

type S3Spec struct {
	BucketName             string
	LifecycleConfiguration *schema.AWSS3BucketLifecycleConfiguration
}

type ResourcesSpec struct {
	DynamoDB []DynamoDBSpec
	S3       []S3Spec
	Layers   []LayerSpec
}
