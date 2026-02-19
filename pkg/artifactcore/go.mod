module github.com/poruru/edge-serverless-box/pkg/artifactcore

go 1.25.1

require (
	github.com/poruru/edge-serverless-box/pkg/yamlshape v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/poruru/edge-serverless-box/pkg/yamlshape => ../yamlshape
