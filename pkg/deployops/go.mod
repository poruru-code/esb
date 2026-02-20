module github.com/poruru/edge-serverless-box/pkg/deployops

go 1.25.1

require (
	github.com/poruru/edge-serverless-box/pkg/artifactcore v0.0.0
	github.com/poruru/edge-serverless-box/pkg/runtimeimage v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/poruru/edge-serverless-box/pkg/yamlshape v0.0.0 // indirect

replace github.com/poruru/edge-serverless-box/pkg/artifactcore => ../artifactcore

replace github.com/poruru/edge-serverless-box/pkg/runtimeimage => ../runtimeimage

replace github.com/poruru/edge-serverless-box/pkg/yamlshape => ../yamlshape
