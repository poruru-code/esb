package manifest

// ResourcesSpec defines the desired state of resources to be provisioned.
// This is the "Intent" or "Manifest" that the Provisioner will use to apply changes.
//
// NOTE: Keep this package free of parser-specific dependencies.
// The generator/parser layer is responsible for mapping external schemas here.
type ResourcesSpec struct {
	DynamoDB []DynamoDBSpec `yaml:"DynamoDB,omitempty"`
	S3       []S3Spec       `yaml:"S3,omitempty"`
	Layers   []LayerSpec    `yaml:"Layers,omitempty"`
}

// DynamoDBSpec defines the parameters for a DynamoDB table.
type DynamoDBSpec struct {
	TableName              string                         `json:"TableName,omitempty" yaml:"TableName,omitempty"`
	KeySchema              []DynamoDBKeySchema            `json:"KeySchema,omitempty" yaml:"KeySchema,omitempty"`
	AttributeDefinitions   []DynamoDBAttributeDefinition  `json:"AttributeDefinitions,omitempty" yaml:"AttributeDefinitions,omitempty"`
	GlobalSecondaryIndexes []DynamoDBGlobalSecondaryIndex `json:"GlobalSecondaryIndexes,omitempty" yaml:"GlobalSecondaryIndexes,omitempty"`
	BillingMode            string                         `json:"BillingMode,omitempty" yaml:"BillingMode,omitempty"`
	ProvisionedThroughput  *DynamoDBProvisionedThroughput `json:"ProvisionedThroughput,omitempty" yaml:"ProvisionedThroughput,omitempty"`
}

// S3Spec defines the parameters for an S3 bucket.
type S3Spec struct {
	BucketName             string                    `json:"BucketName,omitempty" yaml:"BucketName,omitempty"`
	LifecycleConfiguration *S3LifecycleConfiguration `json:"LifecycleConfiguration,omitempty" yaml:"LifecycleConfiguration,omitempty"`
}

// LayerSpec defines the parameters for a Lambda Layer.
type LayerSpec struct {
	Name                    string   `yaml:"Name"`
	ContentURI              string   `yaml:"ContentURI"`
	CompatibleArchitectures []string `yaml:"CompatibleArchitectures,omitempty"`
}

// DynamoDBKeySchema captures a DynamoDB key schema element.
type DynamoDBKeySchema struct {
	AttributeName any `json:"AttributeName" yaml:"AttributeName"`
	KeyType       any `json:"KeyType" yaml:"KeyType"`
}

// DynamoDBAttributeDefinition captures a DynamoDB attribute definition.
type DynamoDBAttributeDefinition struct {
	AttributeName any `json:"AttributeName" yaml:"AttributeName"`
	AttributeType any `json:"AttributeType" yaml:"AttributeType"`
}

// DynamoDBProvisionedThroughput captures provisioned throughput settings.
type DynamoDBProvisionedThroughput struct {
	ReadCapacityUnits  any `json:"ReadCapacityUnits" yaml:"ReadCapacityUnits"`
	WriteCapacityUnits any `json:"WriteCapacityUnits" yaml:"WriteCapacityUnits"`
}

// DynamoDBProjection captures projection settings for indexes.
type DynamoDBProjection struct {
	NonKeyAttributes []any `json:"NonKeyAttributes,omitempty" yaml:"NonKeyAttributes,omitempty"`
	ProjectionType   any   `json:"ProjectionType,omitempty" yaml:"ProjectionType,omitempty"`
}

// DynamoDBGlobalSecondaryIndex captures a GSI definition.
type DynamoDBGlobalSecondaryIndex struct {
	IndexName             any                            `json:"IndexName" yaml:"IndexName"`
	KeySchema             []DynamoDBKeySchema            `json:"KeySchema,omitempty" yaml:"KeySchema,omitempty"`
	Projection            *DynamoDBProjection            `json:"Projection,omitempty" yaml:"Projection,omitempty"`
	ProvisionedThroughput *DynamoDBProvisionedThroughput `json:"ProvisionedThroughput,omitempty" yaml:"ProvisionedThroughput,omitempty"`
}

// S3LifecycleConfiguration captures bucket lifecycle rules.
type S3LifecycleConfiguration struct {
	Rules []S3LifecycleRule `json:"Rules" yaml:"Rules"`
}

// S3LifecycleRule captures the subset of lifecycle rule fields we apply.
type S3LifecycleRule struct {
	ID               any `json:"Id,omitempty" yaml:"Id,omitempty"`
	Status           any `json:"Status" yaml:"Status"`
	Prefix           any `json:"Prefix,omitempty" yaml:"Prefix,omitempty"`
	ExpirationInDays any `json:"ExpirationInDays,omitempty" yaml:"ExpirationInDays,omitempty"`
}
