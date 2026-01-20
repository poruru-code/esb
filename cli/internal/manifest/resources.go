package manifest

// ResourcesSpec defines the desired state of resources to be provisioned.
// This is the "Intent" or "Manifest" that the Provisioner will use to apply changes.
//
// NOTE: Keep this package free of parser-specific dependencies.
// The generator/parser layer is responsible for mapping external schemas here.
type ResourcesSpec struct {
	DynamoDB []DynamoDBSpec
	S3       []S3Spec
	Layers   []LayerSpec
}

// DynamoDBSpec defines the parameters for a DynamoDB table.
type DynamoDBSpec struct {
	TableName              string                         `json:"TableName,omitempty"`
	KeySchema              []DynamoDBKeySchema            `json:"KeySchema,omitempty"`
	AttributeDefinitions   []DynamoDBAttributeDefinition  `json:"AttributeDefinitions,omitempty"`
	GlobalSecondaryIndexes []DynamoDBGlobalSecondaryIndex `json:"GlobalSecondaryIndexes,omitempty"`
	BillingMode            string                         `json:"BillingMode,omitempty"`
	ProvisionedThroughput  *DynamoDBProvisionedThroughput `json:"ProvisionedThroughput,omitempty"`
}

// S3Spec defines the parameters for an S3 bucket.
type S3Spec struct {
	BucketName             string                    `json:"BucketName,omitempty"`
	LifecycleConfiguration *S3LifecycleConfiguration `json:"LifecycleConfiguration,omitempty"`
}

// LayerSpec defines the parameters for a Lambda Layer.
type LayerSpec struct {
	Name                    string
	ContentURI              string
	CompatibleArchitectures []string
}

// DynamoDBKeySchema captures a DynamoDB key schema element.
type DynamoDBKeySchema struct {
	AttributeName any `json:"AttributeName"`
	KeyType       any `json:"KeyType"`
}

// DynamoDBAttributeDefinition captures a DynamoDB attribute definition.
type DynamoDBAttributeDefinition struct {
	AttributeName any `json:"AttributeName"`
	AttributeType any `json:"AttributeType"`
}

// DynamoDBProvisionedThroughput captures provisioned throughput settings.
type DynamoDBProvisionedThroughput struct {
	ReadCapacityUnits  any `json:"ReadCapacityUnits"`
	WriteCapacityUnits any `json:"WriteCapacityUnits"`
}

// DynamoDBProjection captures projection settings for indexes.
type DynamoDBProjection struct {
	NonKeyAttributes []any `json:"NonKeyAttributes,omitempty"`
	ProjectionType   any   `json:"ProjectionType,omitempty"`
}

// DynamoDBGlobalSecondaryIndex captures a GSI definition.
type DynamoDBGlobalSecondaryIndex struct {
	IndexName             any                            `json:"IndexName"`
	KeySchema             []DynamoDBKeySchema            `json:"KeySchema,omitempty"`
	Projection            *DynamoDBProjection            `json:"Projection,omitempty"`
	ProvisionedThroughput *DynamoDBProvisionedThroughput `json:"ProvisionedThroughput,omitempty"`
}

// S3LifecycleConfiguration captures bucket lifecycle rules.
type S3LifecycleConfiguration struct {
	Rules []S3LifecycleRule `json:"Rules"`
}

// S3LifecycleRule captures the subset of lifecycle rule fields we apply.
type S3LifecycleRule struct {
	ID               any `json:"Id,omitempty"`
	Status           any `json:"Status"`
	Prefix           any `json:"Prefix,omitempty"`
	ExpirationInDays any `json:"ExpirationInDays,omitempty"`
}
