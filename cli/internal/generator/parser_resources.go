// Where: cli/internal/generator/parser_resources.go
// What: Resource parsing helpers for SAM templates.
// Why: Separate resource extraction from the main parser flow.
package generator

import "strings"

func parseLayerResources(resources map[string]any, parameters map[string]string) (map[string]LayerSpec, []LayerSpec) {
	layerMap := map[string]LayerSpec{}
	var layers []LayerSpec

	for logicalID, resource := range resources {
		m := asMap(resource)
		if m == nil || asString(m["Type"]) != "AWS::Serverless::LayerVersion" {
			continue
		}
		props := asMap(m["Properties"])
		if props == nil {
			continue
		}
		layerName := asStringDefault(props["LayerName"], logicalID)
		layerName = resolveIntrinsic(layerName, parameters)
		contentURI := asStringDefault(props["ContentUri"], "./")
		contentURI = resolveIntrinsic(contentURI, parameters)
		contentURI = ensureTrailingSlash(contentURI)

		var compatibleArchs []string
		if archs := asSlice(props["CompatibleArchitectures"]); archs != nil {
			for _, a := range archs {
				compatibleArchs = append(compatibleArchs, asString(a))
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

func parseOtherResources(resources map[string]any, parameters map[string]string) ResourcesSpec {
	parsed := ResourcesSpec{}

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
			tableName := asStringDefault(props["TableName"], logicalID)
			tableName = resolveIntrinsic(tableName, parameters)
			parsed.DynamoDB = append(parsed.DynamoDB, DynamoDBSpec{
				TableName:              tableName,
				KeySchema:              props["KeySchema"],
				AttributeDefinitions:   props["AttributeDefinitions"],
				GlobalSecondaryIndexes: props["GlobalSecondaryIndexes"],
				BillingMode:            asStringDefault(props["BillingMode"], "PROVISIONED"),
				ProvisionedThroughput:  props["ProvisionedThroughput"],
			})
		case "AWS::S3::Bucket":
			bucketName := asStringDefault(props["BucketName"], strings.ToLower(logicalID))
			bucketName = resolveIntrinsic(bucketName, parameters)
			parsed.S3 = append(parsed.S3, S3Spec{
				BucketName:             bucketName,
				LifecycleConfiguration: props["LifecycleConfiguration"],
			})

		}
	}

	return parsed
}
