module github.com/poruru-code/esb/pkg/artifactcore

go 1.25.1

require (
	github.com/poruru-code/esb/pkg/yamlshape v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/poruru-code/esb/pkg/yamlshape => ../yamlshape
