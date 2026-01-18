// Where: cli/internal/generator/parser_resources.go
// What: Resource parsing helpers for SAM templates.
// Why: Separate resource extraction from the main parser flow.
package generator

import (
	"fmt"

	samparser "github.com/poruru-code/aws-sam-parser-go/parser"
	"github.com/poruru-code/aws-sam-parser-go/schema"
	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
)

func parseLayerResources(resources map[string]any) (map[string]manifest.LayerSpec, []manifest.LayerSpec) {
	layerMap := map[string]manifest.LayerSpec{}
	var layers []manifest.LayerSpec

	for logicalID, resource := range resources {
		m := asMap(resource)
		if m == nil || asString(m["Type"]) != "AWS::Serverless::LayerVersion" {
			continue
		}
		props := asMap(m["Properties"])
		if props == nil {
			continue
		}
		layerName := ResolveFunctionName(props["LayerName"], logicalID)
		contentURI := ResolveCodeURI(props["ContentUri"])
		contentURI = ensureTrailingSlash(contentURI)

		var compatibleArchs []string
		if archs := asSlice(props["CompatibleArchitectures"]); archs != nil {
			for _, a := range archs {
				compatibleArchs = append(compatibleArchs, asString(a))
			}
		}

		spec := manifest.LayerSpec{
			Name:                    layerName,
			ContentURI:              contentURI,
			CompatibleArchitectures: compatibleArchs,
		}
		layerMap[logicalID] = spec
		layers = append(layers, spec)
	}

	return layerMap, layers
}

func parseOtherResources(resources map[string]any) manifest.ResourcesSpec {
	parsed := manifest.ResourcesSpec{}

	for logicalID, value := range resources {
		resource := asMap(value)
		if resource == nil {
			continue
		}
		resourceType := asString(resource["Type"])
		if resourceType == "AWS::Serverless::LayerVersion" || resourceType == "AWS::Serverless::Function" {
			continue
		}
		props := asMap(resource["Properties"])

		switch resourceType {
		case "AWS::DynamoDB::Table":
			tableName := ResolveTableName(props, logicalID)

			var tableProps schema.AWSDynamoDBTableProperties
			if err := samparser.Decode(props, &tableProps, nil); err != nil {
				fmt.Printf("Warning: failed to map DynamoDB table %s: %v\n", logicalID, err)
			}

			parsed.DynamoDB = append(parsed.DynamoDB, manifest.DynamoDBSpec{
				TableName:              tableName,
				KeySchema:              tableProps.KeySchema,
				AttributeDefinitions:   tableProps.AttributeDefinitions,
				GlobalSecondaryIndexes: tableProps.GlobalSecondaryIndexes,
				BillingMode:            ResolveBillingMode(props),
				ProvisionedThroughput:  tableProps.ProvisionedThroughput,
			})
		case "AWS::S3::Bucket":
			bucketName := ResolveS3BucketName(props, logicalID)

			var s3Props schema.AWSS3BucketProperties
			if err := samparser.Decode(props, &s3Props, nil); err != nil {
				fmt.Printf("Warning: failed to map S3 bucket %s: %v\n", logicalID, err)
			}

			parsed.S3 = append(parsed.S3, manifest.S3Spec{
				BucketName:             bucketName,
				LifecycleConfiguration: s3Props.LifecycleConfiguration,
			})

		}
	}

	return parsed
}
