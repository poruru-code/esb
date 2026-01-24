// Where: cli/internal/provisioner/dynamodb.go
// What: DynamoDB provisioning helpers.
// Why: Create DynamoDB-compatible tables based on SAM resources.
package provisioner

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/poruru-code/aws-sam-parser-go/schema"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
)

type DynamoDBAPI interface {
	ListTables(ctx context.Context) ([]string, error)
	CreateTable(ctx context.Context, input DynamoCreateInput) error
}

type DynamoCreateInput struct {
	TableName              string
	KeySchema              []KeySchemaElement
	AttributeDefinitions   []AttributeDefinition
	BillingMode            string
	ProvisionedThroughput  *ProvisionedThroughput
	GlobalSecondaryIndexes []GlobalSecondaryIndex
}

type KeySchemaElement struct {
	AttributeName string
	KeyType       string
}

type AttributeDefinition struct {
	AttributeName string
	AttributeType string
}

type ProvisionedThroughput struct {
	ReadCapacityUnits  int64
	WriteCapacityUnits int64
}

type Projection struct {
	ProjectionType   string
	NonKeyAttributes []string
}

type GlobalSecondaryIndex struct {
	IndexName             string
	KeySchema             []KeySchemaElement
	Projection            Projection
	ProvisionedThroughput *ProvisionedThroughput
}

func provisionDynamo(
	ctx context.Context,
	client DynamoDBAPI,
	tables []manifest.DynamoDBSpec,
	out io.Writer,
) {
	if client == nil || len(tables) == 0 {
		return
	}
	if out == nil {
		out = io.Discard
	}

	existingTables := map[string]struct{}{}
	if names, err := client.ListTables(ctx); err == nil {
		for _, name := range names {
			existingTables[name] = struct{}{}
		}
	}

	for _, table := range tables {
		name := strings.TrimSpace(table.TableName)
		if name == "" {
			continue
		}
		if _, ok := existingTables[name]; ok {
			fmt.Fprintf(out, "Table '%s' already exists. Skipping.\n", name)
			continue
		}

		fmt.Fprintf(out, "Creating DynamoDB Table: %s\n", name)
		input, err := buildDynamoCreateInput(table)
		if err != nil {
			fmt.Fprintf(out, "❌ Failed to create table %s: %v\n", name, err)
			continue
		}
		if err := client.CreateTable(ctx, input); err != nil {
			fmt.Fprintf(out, "❌ Failed to create table %s: %v\n", name, err)
			continue
		}
		fmt.Fprintf(out, "✅ Created DynamoDB Table: %s\n", name)
	}
}

func buildDynamoCreateInput(table manifest.DynamoDBSpec) (DynamoCreateInput, error) {
	keySchema, err := parseKeySchema(table.KeySchema)
	if err != nil {
		return DynamoCreateInput{}, err
	}
	attributeDefinitions, err := parseAttributeDefinitions(table.AttributeDefinitions)
	if err != nil {
		return DynamoCreateInput{}, err
	}

	billingMode := strings.ToUpper(strings.TrimSpace(table.BillingMode))
	if billingMode == "" {
		billingMode = "PROVISIONED"
	}

	throughput, err := parseProvisionedThroughput(table.ProvisionedThroughput, billingMode)
	if err != nil {
		return DynamoCreateInput{}, err
	}

	gsis, err := parseGlobalSecondaryIndexes(table.GlobalSecondaryIndexes, billingMode)
	if err != nil {
		return DynamoCreateInput{}, err
	}

	return DynamoCreateInput{
		TableName:              table.TableName,
		KeySchema:              keySchema,
		AttributeDefinitions:   attributeDefinitions,
		BillingMode:            billingMode,
		ProvisionedThroughput:  throughput,
		GlobalSecondaryIndexes: gsis,
	}, nil
}

func parseKeySchema(items []schema.AWSDynamoDBTableKeySchema) ([]KeySchemaElement, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]KeySchemaElement, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(toString(item.AttributeName))
		keyType := strings.TrimSpace(toString(item.KeyType))
		out = append(out, KeySchemaElement{AttributeName: name, KeyType: keyType})
	}
	return out, nil
}

func parseAttributeDefinitions(items []schema.AWSDynamoDBTableAttributeDefinition) ([]AttributeDefinition, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]AttributeDefinition, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(toString(item.AttributeName))
		attrType := strings.TrimSpace(toString(item.AttributeType))
		out = append(out, AttributeDefinition{AttributeName: name, AttributeType: attrType})
	}
	return out, nil
}

func parseProvisionedThroughput(item *schema.AWSDynamoDBTableProvisionedThroughput, billingMode string) (*ProvisionedThroughput, error) {
	if strings.EqualFold(billingMode, "PAY_PER_REQUEST") {
		return nil, nil
	}
	if item == nil {
		return &ProvisionedThroughput{ReadCapacityUnits: 1, WriteCapacityUnits: 1}, nil
	}

	readUnits, err := toInt64(item.ReadCapacityUnits)
	if err != nil && item.ReadCapacityUnits != nil {
		return nil, fmt.Errorf("invalid ReadCapacityUnits: %w", err)
	}
	if readUnits == 0 {
		readUnits = 1
	}

	writeUnits, err := toInt64(item.WriteCapacityUnits)
	if err != nil && item.WriteCapacityUnits != nil {
		return nil, fmt.Errorf("invalid WriteCapacityUnits: %w", err)
	}
	if writeUnits == 0 {
		writeUnits = 1
	}
	return &ProvisionedThroughput{
		ReadCapacityUnits:  readUnits,
		WriteCapacityUnits: writeUnits,
	}, nil
}

func parseGlobalSecondaryIndexes(items []schema.AWSDynamoDBTableGlobalSecondaryIndex, billingMode string) ([]GlobalSecondaryIndex, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]GlobalSecondaryIndex, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(toString(item.IndexName))
		keySchema, err := parseKeySchema(item.KeySchema)
		if err != nil {
			return nil, err
		}
		projection, err := parseProjection(item.Projection)
		if err != nil {
			return nil, err
		}
		throughput, err := parseProvisionedThroughput(item.ProvisionedThroughput, billingMode)
		if err != nil {
			return nil, err
		}
		out = append(out, GlobalSecondaryIndex{
			IndexName:             name,
			KeySchema:             keySchema,
			Projection:            projection,
			ProvisionedThroughput: throughput,
		})
	}
	return out, nil
}

func parseProjection(item *schema.AWSDynamoDBTableProjection) (Projection, error) {
	if item == nil {
		return Projection{}, nil
	}
	projectionType := strings.TrimSpace(toString(item.ProjectionType))

	// NonKeyAttributes is []interface{} in schema because of strict parsing of []interface{}?
	// In sam_generated.go: NonKeyAttributes []interface{} `json:"NonKeyAttributes,omitempty"`
	// So we need to convert slice of interface to slice of string.
	nonKeys := toStringSlice(item.NonKeyAttributes)

	return Projection{
		ProjectionType:   projectionType,
		NonKeyAttributes: nonKeys,
	}, nil
}
