// Where: cli/internal/generator/parser_resources.go
// What: Resource parsing helpers for SAM templates.
// Why: Separate resource extraction from the main parser flow.
package generator

import (
	"fmt"

	"github.com/poruru/edge-serverless-box/cli/internal/generator/schema"
)

func parseLayerResources(resources map[string]any, ctx *ParserContext) (map[string]LayerSpec, []LayerSpec) {
	layerMap := map[string]LayerSpec{}
	var layers []LayerSpec

	for logicalID, resource := range resources {
		m := ctx.asMap(resource)
		if m == nil || ctx.asString(m["Type"]) != "AWS::Serverless::LayerVersion" {
			continue
		}
		props := ctx.asMap(m["Properties"])
		if props == nil {
			continue
		}
		layerName := ResolveFunctionName(props["LayerName"], logicalID, ctx)
		contentURI := ResolveCodeURI(props["ContentUri"], ctx)
		contentURI = ensureTrailingSlash(contentURI)

		var compatibleArchs []string
		if archs := ctx.asSlice(props["CompatibleArchitectures"]); archs != nil {
			for _, a := range archs {
				compatibleArchs = append(compatibleArchs, ctx.asString(a))
			}
		}

		spec := LayerSpec{
			Name:                    layerName,
			ContentURI:              contentURI,
			CompatibleArchitectures: compatibleArchs,
		}
		layerMap[logicalID] = spec
		layers = append(layers, spec)
	}

	return layerMap, layers
}

func parseOtherResources(resources map[string]any, ctx *ParserContext) ResourcesSpec {
	parsed := ResourcesSpec{}

	for logicalID, value := range resources {
		resource := ctx.asMap(value)
		if resource == nil {
			continue
		}
		resourceType := ctx.asString(resource["Type"])
		if resourceType == "AWS::Serverless::LayerVersion" || resourceType == "AWS::Serverless::Function" {
			continue
		}
		props := ctx.asMap(resource["Properties"])

		switch resourceType {
		case "AWS::DynamoDB::Table":
			tableName := ResolveTableName(props, logicalID, ctx)

			var tableProps schema.AWSDynamoDBTableProperties
			if err := ctx.mapToStruct(props, &tableProps); err != nil {
				fmt.Printf("Warning: failed to map DynamoDB table %s: %v\n", logicalID, err)
			}

			parsed.DynamoDB = append(parsed.DynamoDB, DynamoDBSpec{
				TableName:              tableName,
				KeySchema:              tableProps.KeySchema,
				AttributeDefinitions:   tableProps.AttributeDefinitions,
				GlobalSecondaryIndexes: tableProps.GlobalSecondaryIndexes,
				BillingMode:            ResolveBillingMode(props, ctx),
				ProvisionedThroughput:  tableProps.ProvisionedThroughput,
			})
		case "AWS::S3::Bucket":
			bucketName := ResolveS3BucketName(props, logicalID, ctx)

			var s3Props schema.AWSS3BucketProperties
			if err := ctx.mapToStruct(props, &s3Props); err != nil {
				fmt.Printf("Warning: failed to map S3 bucket %s: %v\n", logicalID, err)
			}

			parsed.S3 = append(parsed.S3, S3Spec{
				BucketName:             bucketName,
				LifecycleConfiguration: s3Props.LifecycleConfiguration,
			})

		}
	}

	return parsed
}
