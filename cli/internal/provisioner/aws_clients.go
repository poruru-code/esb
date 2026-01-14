// Where: cli/internal/provisioner/aws_clients.go
// What: AWS SDK adapters for DynamoDB and S3.
// Why: Map internal provisioner types to SDK types.
package provisioner

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/poruru/edge-serverless-box/cli/internal/generator/schema"
)

type awsDynamoClient struct {
	client *dynamodb.Client
}

func (c awsDynamoClient) ListTables(ctx context.Context) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("dynamodb client is nil")
	}
	resp, err := c.client.ListTables(ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		return nil, err
	}
	return resp.TableNames, nil
}

func (c awsDynamoClient) CreateTable(ctx context.Context, input DynamoCreateInput) error {
	if c.client == nil {
		return fmt.Errorf("dynamodb client is nil")
	}
	awsInput, err := buildAWSCreateTableInput(input)
	if err != nil {
		return err
	}
	_, err = c.client.CreateTable(ctx, awsInput)
	return err
}

func buildAWSCreateTableInput(input DynamoCreateInput) (*dynamodb.CreateTableInput, error) {
	billingMode, err := mapBillingMode(input.BillingMode)
	if err != nil {
		return nil, err
	}
	keySchema, err := mapKeySchema(input.KeySchema)
	if err != nil {
		return nil, err
	}
	attrDefs, err := mapAttributeDefinitions(input.AttributeDefinitions)
	if err != nil {
		return nil, err
	}

	out := &dynamodb.CreateTableInput{
		TableName:            aws.String(input.TableName),
		KeySchema:            keySchema,
		AttributeDefinitions: attrDefs,
		BillingMode:          billingMode,
	}
	if input.ProvisionedThroughput != nil && billingMode == types.BillingModeProvisioned {
		out.ProvisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(input.ProvisionedThroughput.ReadCapacityUnits),
			WriteCapacityUnits: aws.Int64(input.ProvisionedThroughput.WriteCapacityUnits),
		}
	}
	if len(input.GlobalSecondaryIndexes) > 0 {
		gsis, err := mapGlobalSecondaryIndexes(input.GlobalSecondaryIndexes, billingMode)
		if err != nil {
			return nil, err
		}
		out.GlobalSecondaryIndexes = gsis
	}
	return out, nil
}

func mapBillingMode(value string) (types.BillingMode, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PAY_PER_REQUEST":
		return types.BillingModePayPerRequest, nil
	case "PROVISIONED", "":
		return types.BillingModeProvisioned, nil
	default:
		return "", fmt.Errorf("unsupported billing mode: %s", value)
	}
}

func mapKeySchema(values []KeySchemaElement) ([]types.KeySchemaElement, error) {
	out := make([]types.KeySchemaElement, 0, len(values))
	for _, item := range values {
		keyType, err := mapKeyType(item.KeyType)
		if err != nil {
			return nil, err
		}
		out = append(out, types.KeySchemaElement{
			AttributeName: aws.String(item.AttributeName),
			KeyType:       keyType,
		})
	}
	return out, nil
}

func mapKeyType(value string) (types.KeyType, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "HASH":
		return types.KeyTypeHash, nil
	case "RANGE":
		return types.KeyTypeRange, nil
	default:
		return "", fmt.Errorf("unsupported key type: %s", value)
	}
}

func mapAttributeDefinitions(values []AttributeDefinition) ([]types.AttributeDefinition, error) {
	out := make([]types.AttributeDefinition, 0, len(values))
	for _, item := range values {
		attrType, err := mapAttributeType(item.AttributeType)
		if err != nil {
			return nil, err
		}
		out = append(out, types.AttributeDefinition{
			AttributeName: aws.String(item.AttributeName),
			AttributeType: attrType,
		})
	}
	return out, nil
}

func mapAttributeType(value string) (types.ScalarAttributeType, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "S":
		return types.ScalarAttributeTypeS, nil
	case "N":
		return types.ScalarAttributeTypeN, nil
	case "B":
		return types.ScalarAttributeTypeB, nil
	default:
		return "", fmt.Errorf("unsupported attribute type: %s", value)
	}
}

func mapGlobalSecondaryIndexes(
	values []GlobalSecondaryIndex,
	billingMode types.BillingMode,
) ([]types.GlobalSecondaryIndex, error) {
	out := make([]types.GlobalSecondaryIndex, 0, len(values))
	for _, item := range values {
		keySchema, err := mapKeySchema(item.KeySchema)
		if err != nil {
			return nil, err
		}
		projection, err := mapProjection(item.Projection)
		if err != nil {
			return nil, err
		}
		gsi := types.GlobalSecondaryIndex{
			IndexName: aws.String(item.IndexName),
			KeySchema: keySchema,
		}
		if projection != nil {
			gsi.Projection = projection
		}
		if item.ProvisionedThroughput != nil && billingMode == types.BillingModeProvisioned {
			gsi.ProvisionedThroughput = &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(item.ProvisionedThroughput.ReadCapacityUnits),
				WriteCapacityUnits: aws.Int64(item.ProvisionedThroughput.WriteCapacityUnits),
			}
		}
		out = append(out, gsi)
	}
	return out, nil
}

func mapProjection(value Projection) (*types.Projection, error) {
	out := &types.Projection{}
	switch strings.ToUpper(strings.TrimSpace(value.ProjectionType)) {
	case "":
	case "ALL":
		out.ProjectionType = types.ProjectionTypeAll
	case "KEYS_ONLY":
		out.ProjectionType = types.ProjectionTypeKeysOnly
	case "INCLUDE":
		out.ProjectionType = types.ProjectionTypeInclude
	default:
		return nil, fmt.Errorf("unsupported projection type: %s", value.ProjectionType)
	}
	if len(value.NonKeyAttributes) > 0 {
		out.NonKeyAttributes = value.NonKeyAttributes
	}
	if out.ProjectionType == "" && len(out.NonKeyAttributes) == 0 {
		return nil, nil
	}
	return out, nil
}

type awsS3Client struct {
	client *s3.Client
}

func (c awsS3Client) ListBuckets(ctx context.Context) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("s3 client is nil")
	}
	resp, err := c.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Buckets))
	for _, bucket := range resp.Buckets {
		if bucket.Name == nil {
			continue
		}
		names = append(names, *bucket.Name)
	}
	return names, nil
}

func (c awsS3Client) CreateBucket(ctx context.Context, name string) error {
	if c.client == nil {
		return fmt.Errorf("s3 client is nil")
	}
	_, err := c.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)})
	return err
}

// PutBucketLifecycleConfiguration applies lifecycle rules to the bucket.
func (c awsS3Client) PutBucketLifecycleConfiguration(ctx context.Context, name string, config *schema.AWSS3BucketLifecycleConfiguration) error {
	if c.client == nil {
		return fmt.Errorf("s3 client is nil")
	}

	if config == nil {
		return nil
	}

	awsRules, err := mapLifecycleRules(config.Rules)
	if err != nil {
		return err
	}

	// AWS SDK requires at least one rule? Or it can be empty?
	if len(awsRules) == 0 {
		return nil
	}

	_, err = c.client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(name),
		LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
			Rules: awsRules,
		},
	})
	return err
}

func mapLifecycleRules(items []schema.AWSS3BucketRule) ([]s3types.LifecycleRule, error) {
	out := make([]s3types.LifecycleRule, 0, len(items))
	for _, item := range items {
		rule := s3types.LifecycleRule{}
		statusStr := strings.ToLower(toString(item.Status))
		if statusStr == "enabled" {
			rule.Status = s3types.ExpirationStatusEnabled
		} else {
			rule.Status = s3types.ExpirationStatusDisabled
		}

		if id := toString(item.Id); id != "" {
			rule.ID = aws.String(id)
		}

		// Filter
		if item.Prefix != nil {
			if pStr := toString(item.Prefix); pStr != "" {
				rule.Filter = &s3types.LifecycleRuleFilter{Prefix: aws.String(pStr)}
			}
		}

		// Expiration
		if exp := item.ExpirationInDays; exp != nil {
			if days, ok := toInt64(exp); ok {
				rule.Expiration = &s3types.LifecycleExpiration{Days: aws.Int32(int32(days))}
			}
		}

		out = append(out, rule)
	}
	return out, nil
}
