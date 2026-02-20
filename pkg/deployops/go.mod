module github.com/poruru-code/esb/pkg/deployops

go 1.25.1

require (
	github.com/poruru-code/esb/pkg/artifactcore v0.0.0
	github.com/poruru-code/esb/pkg/runtimeimage v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/poruru-code/esb/pkg/yamlshape v0.0.0 // indirect

replace github.com/poruru-code/esb/pkg/artifactcore => ../artifactcore

replace github.com/poruru-code/esb/pkg/runtimeimage => ../runtimeimage

replace github.com/poruru-code/esb/pkg/yamlshape => ../yamlshape
