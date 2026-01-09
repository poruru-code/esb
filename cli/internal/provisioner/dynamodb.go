// Where: cli/internal/provisioner/dynamodb.go
// What: DynamoDB provisioning helpers.
// Why: Create DynamoDB-compatible tables based on SAM resources.
package provisioner

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/generator"
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
	tables []generator.DynamoDBSpec,
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

func buildDynamoCreateInput(table generator.DynamoDBSpec) (DynamoCreateInput, error) {
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

func parseKeySchema(raw any) ([]KeySchemaElement, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid key schema")
	}
	out := make([]KeySchemaElement, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid key schema entry")
		}
		name := strings.TrimSpace(toString(entry["AttributeName"]))
		keyType := strings.TrimSpace(toString(entry["KeyType"]))
		out = append(out, KeySchemaElement{AttributeName: name, KeyType: keyType})
	}
	return out, nil
}

func parseAttributeDefinitions(raw any) ([]AttributeDefinition, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid attribute definitions")
	}
	out := make([]AttributeDefinition, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid attribute definition entry")
		}
		name := strings.TrimSpace(toString(entry["AttributeName"]))
		attrType := strings.TrimSpace(toString(entry["AttributeType"]))
		out = append(out, AttributeDefinition{AttributeName: name, AttributeType: attrType})
	}
	return out, nil
}

func parseProvisionedThroughput(raw any, billingMode string) (*ProvisionedThroughput, error) {
	if strings.EqualFold(billingMode, "PAY_PER_REQUEST") {
		return nil, nil
	}
	if raw == nil {
		return &ProvisionedThroughput{ReadCapacityUnits: 1, WriteCapacityUnits: 1}, nil
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid provisioned throughput")
	}
	readUnits, ok := toInt64(entry["ReadCapacityUnits"])
	if !ok {
		readUnits = 1
	}
	writeUnits, ok := toInt64(entry["WriteCapacityUnits"])
	if !ok {
		writeUnits = 1
	}
	return &ProvisionedThroughput{
		ReadCapacityUnits:  readUnits,
		WriteCapacityUnits: writeUnits,
	}, nil
}

func parseGlobalSecondaryIndexes(raw any, billingMode string) ([]GlobalSecondaryIndex, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid global secondary indexes")
	}
	out := make([]GlobalSecondaryIndex, 0, len(values))
	for _, value := range values {
		entry, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid global secondary index entry")
		}
		name := strings.TrimSpace(toString(entry["IndexName"]))
		keySchema, err := parseKeySchema(entry["KeySchema"])
		if err != nil {
			return nil, err
		}
		projection, err := parseProjection(entry["Projection"])
		if err != nil {
			return nil, err
		}
		throughput, err := parseProvisionedThroughput(entry["ProvisionedThroughput"], billingMode)
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

func parseProjection(raw any) (Projection, error) {
	if raw == nil {
		return Projection{}, nil
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		return Projection{}, fmt.Errorf("invalid projection")
	}
	projectionType := strings.TrimSpace(toString(entry["ProjectionType"]))
	nonKeys := toStringSlice(entry["NonKeyAttributes"])
	return Projection{
		ProjectionType:   projectionType,
		NonKeyAttributes: nonKeys,
	}, nil
}
