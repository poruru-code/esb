// Where: cli/internal/infra/sam/template_resources.go
// What: Resource parsing helpers for SAM templates.
// Why: Separate resource extraction from the main parser flow.
package sam

import (
	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

func parseLayerResources(resources map[string]any) (map[string]manifest.LayerSpec, []manifest.LayerSpec) {
	layerMap := map[string]manifest.LayerSpec{}
	var layers []manifest.LayerSpec

	for _, logicalID := range sortedMapKeys(resources) {
		resource := resources[logicalID]
		m := value.AsMap(resource)
		if m == nil || value.AsString(m["Type"]) != "AWS::Serverless::LayerVersion" {
			continue
		}
		props := value.AsMap(m["Properties"])
		if props == nil {
			continue
		}
		layerName := ResolveFunctionName(props["LayerName"], logicalID)
		contentURI := ResolveCodeURI(props["ContentUri"])
		contentURI = value.EnsureTrailingSlash(contentURI)

		var compatibleArchs []string
		if archs := value.AsSlice(props["CompatibleArchitectures"]); archs != nil {
			for _, a := range archs {
				compatibleArchs = append(compatibleArchs, value.AsString(a))
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

func parseOtherResources(resources map[string]any, warnf func(string, ...any)) manifest.ResourcesSpec {
	parsed := manifest.ResourcesSpec{}

	for _, logicalID := range sortedMapKeys(resources) {
		raw := resources[logicalID]
		resource := value.AsMap(raw)
		if resource == nil {
			continue
		}
		resourceType := value.AsString(resource["Type"])
		if resourceType == "AWS::Serverless::LayerVersion" || resourceType == "AWS::Serverless::Function" {
			continue
		}
		props := value.AsMap(resource["Properties"])

		switch resourceType {
		case "AWS::DynamoDB::Table":
			tableName := ResolveTableName(props, logicalID)

			tableProps, err := DecodeDynamoDBProps(props)
			if err != nil && warnf != nil {
				warnf("failed to map DynamoDB table %s: %v", logicalID, err)
			}

			tableProps.TableName = tableName
			tableProps.BillingMode = ResolveBillingMode(props)
			parsed.DynamoDB = append(parsed.DynamoDB, tableProps)
		case "AWS::S3::Bucket":
			bucketName := ResolveS3BucketName(props, logicalID)

			s3Props, err := DecodeS3BucketProps(props)
			if err != nil && warnf != nil {
				warnf("failed to map S3 bucket %s: %v", logicalID, err)
			}

			s3Props.BucketName = bucketName
			parsed.S3 = append(parsed.S3, s3Props)
		}
	}

	return parsed
}
